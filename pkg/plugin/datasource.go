package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/emnify/pcap-extractor/pkg/models"
	"github.com/grafana/grafana-aws-sdk/pkg/awsauth"
	"github.com/grafana/grafana-aws-sdk/pkg/awsds"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// Define separate interfaces to facilitate mocking in tests
type SFNClientInterface interface {
	StartExecution(ctx context.Context, params *sfn.StartExecutionInput, optFns ...func(*sfn.Options)) (*sfn.StartExecutionOutput, error)
	DescribeExecution(ctx context.Context, params *sfn.DescribeExecutionInput, optFns ...func(*sfn.Options)) (*sfn.DescribeExecutionOutput, error)
	DescribeStateMachine(ctx context.Context, params *sfn.DescribeStateMachineInput, optFns ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error)
}

type S3PresignerInterface interface {
	PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

func NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	backend.Logger.Info("Creating new Datasource pcap-extractor")

	// Load plugin-specific settings
	pluginSettings, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin settings: %w", err)
	}

	// Initialize AWS datasource settings using Grafana AWS SDK
	awsDS := &awsds.AWSDatasourceSettings{}
	err = awsDS.Load(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS datasource settings: %w", err)
	}

	// Create AWS auth config provider
	authConfig := awsauth.NewConfigProvider()

	// Convert to awsauth.Settings
	authSettings := awsauth.Settings{
		AccessKey:          awsDS.AccessKey,
		SecretKey:          awsDS.SecretKey,
		CredentialsProfile: awsDS.Profile,
		AssumeRoleARN:      awsDS.AssumeRoleARN,
		ExternalID:         awsDS.ExternalID,
		Endpoint:           awsDS.Endpoint,
		Region:             awsDS.Region,
		HTTPClient:         &http.Client{},
	}

	// Get AWS config using Grafana AWS SDK
	cfg, err := authConfig.GetConfig(ctx, authSettings)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS config: %w", err)
	}

	// Create Step Functions client
	sfnClient := sfn.NewFromConfig(cfg)

	// Create S3 client
	s3Client := s3.NewFromConfig(cfg)

	return &Datasource{
		settings:          pluginSettings,
		AWSConfigProvider: awsauth.NewConfigProvider(),
		sfnClient:         sfnClient,
		s3Client:          s3Client,
		s3Presigner:       s3.NewPresignClient(s3Client),
	}, nil
}

type Datasource struct {
	AWSConfigProvider awsauth.ConfigProvider
	settings          *models.PluginSettings
	sfnClient         SFNClientInterface
	s3Client          *s3.Client
	s3Presigner       S3PresignerInterface
}

type queryModel struct {
	Action  string           `json:"action"`
	JobId   string           `json:"JobId"`
	Extract map[string][]int `json:"Extract"` // only for action=request
}

type StepFunctionInput struct {
	JobId   string           `json:"jobId"`
	Bucket  string           `json:"bucket"`
	Extract map[string][]int `json:"extract"`
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created. As soon as datasource settings change detected by SDK old datasource instance will
// be disposed and a new one will be created using NewSampleDatasource factory function.
func (d *Datasource) Dispose() {
	// Clean up datasource instance resources.
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifier).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	// create response struct
	response := backend.NewQueryDataResponse()

	// loop over queries and execute them individually.
	for _, q := range req.Queries {
		res := d.query(ctx, req.PluginContext, q)

		// save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = res
	}

	return response, nil
}

func (d *Datasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	if err := d.validateSettings(ctx); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("Incomplete plugin settings: %v", err.Error()))
	}

	// Unmarshal the JSON into our queryModel.
	var qm queryModel

	err := json.Unmarshal(query.JSON, &qm)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err.Error()))
	}

	switch qm.Action {
	case "request":
		return d.handleRequestAction(ctx, qm)
	case "status":
		return d.handleStatusAction(ctx, qm)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown action: '%s'", qm.Action))
	}
}

func (d *Datasource) handleRequestAction(ctx context.Context, qm queryModel) backend.DataResponse {
	var response backend.DataResponse

	// Check if we have extract data to process
	if len(qm.Extract) == 0 {
		return backend.ErrDataResponse(backend.StatusBadRequest, "Extract parameter is required for request action")
	}

	if qm.JobId == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "JobId is required for request action")
	}

	backend.Logger.Info("Processing request action", "jobId", qm.JobId, "extract", qm.Extract)

	sfnInput := StepFunctionInput{
		JobId:   qm.JobId,
		Extract: qm.Extract,
		Bucket:  d.settings.S3Bucket,
	}

	// Call Step Function
	executionArn, err := d.executeStepFunction(ctx, qm.JobId, sfnInput)
	if err != nil {
		backend.Logger.Error("Failed to execute Step Function", "error", err)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("Step Function execution failed: %v", err.Error()))
	}

	backend.Logger.Debug("Step Function executed successfully", "executionArn", executionArn)

	// Create response frame with execution information
	frame := data.NewFrame("step_function_request")
	frame.Fields = append(frame.Fields,
		data.NewField("status", nil, []string{"RUNNING"}), // we assume it worked
		data.NewField("job_id", nil, []string{qm.JobId}),
	)

	response.Frames = append(response.Frames, frame)
	return response
}

func (d *Datasource) handleStatusAction(ctx context.Context, qm queryModel) backend.DataResponse {
	var response backend.DataResponse

	if qm.JobId == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "JobId is required for status action")
	}

	backend.Logger.Info("Processing status action", "jobId", qm.JobId)

	arn, err := arn.Parse(d.settings.StepFunctionArn)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("Failed to parse Step Function ARN: %v", err.Error()))
	}

	// examplearn:aws:states:eu-west-1:123456789012:execution:my-pcap-extractor:run-1761774923333
	executionArn := fmt.Sprintf("arn:aws:states:%v:%v:execution:%v:%v", arn.Region, arn.AccountID, strings.Replace(arn.Resource, "stateMachine:", "", 1), qm.JobId)
	backend.Logger.Info("Trying to describe Step Function execution", "arn", executionArn)

	// Get execution status from Step Functions
	result, err := d.sfnClient.DescribeExecution(ctx, &sfn.DescribeExecutionInput{
		ExecutionArn: &executionArn,
	})
	if err != nil {
		backend.Logger.Error("Failed to describe Step Function execution", "error", err)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("Failed to get execution status: %v", err.Error()))
	}

	status := string(result.Status)
	backend.Logger.Info("Step Function execution status", "status", status, "executionArn", executionArn)

	// Create response frame with status information
	frame := data.NewFrame("step_function_status")
	frame.Fields = append(frame.Fields,
		data.NewField("status", nil, []string{status}),
	)

	// Add error information if execution failed
	if status == "FAILED" && result.Error != nil {
		frame.Fields = append(frame.Fields,
			data.NewField("error", nil, []string{*result.Error}),
		)
	}

	// Add cause information if available
	if result.Cause != nil {
		frame.Fields = append(frame.Fields,
			data.NewField("cause", nil, []string{*result.Cause}),
		)
	}

	// If execution is successful, generate presigned URL
	if status == "SUCCEEDED" {
		s3Key := fmt.Sprintf("%s.pcapng", qm.JobId)
		presignedURL, err := d.generatePresignedURL(ctx, d.settings.S3Bucket, s3Key)
		if err != nil {
			backend.Logger.Warn("Failed to generate presigned URL for completed execution", "error", err)
		} else {
			frame.Fields = append(frame.Fields,
				data.NewField("download_url", nil, []string{presignedURL}),
			)
		}
	}

	response.Frames = append(response.Frames, frame)
	return response
}

func (d *Datasource) executeStepFunction(ctx context.Context, name string, input StepFunctionInput) (string, error) {

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Step Function input: %w", err)
	}

	// Execute the Step Function
	inputStr := string(inputJSON)
	result, err := d.sfnClient.StartExecution(ctx, &sfn.StartExecutionInput{
		Name:            &name,
		StateMachineArn: &d.settings.StepFunctionArn,
		Input:           &inputStr,
	})

	if err != nil {
		return "", fmt.Errorf("failed to execute Step Function execution: %w", err)
	}

	return *result.ExecutionArn, nil
}

func (d *Datasource) generatePresignedURL(ctx context.Context, bucket, key string) (string, error) {
	if d.s3Presigner == nil {
		return "", fmt.Errorf("S3 presigner is not initialized")
	}

	// Generate presigned URL for GetObject with 1 hour expiration
	request, err := d.s3Presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, func(opts *s3.PresignOptions) {
		opts.Expires = time.Hour * 1
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return request.URL, nil
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	res := &backend.CheckHealthResult{}
	var messages []string

	// Check Step Function ARN configuration
	if d.settings.S3Bucket == "" {
		res.Status = backend.HealthStatusError
		res.Message = "S3 Bucket name is missing"
		return res, nil
	}

	// Check Step Function ARN configuration
	if d.settings.StepFunctionArn == "" {
		res.Status = backend.HealthStatusError
		res.Message = "Step Function ARN is missing"
		return res, nil
	}

	// Test Step Function accessibility
	if d.sfnClient != nil {
		_, err := d.sfnClient.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
			StateMachineArn: &d.settings.StepFunctionArn,
		})
		if err != nil {
			res.Status = backend.HealthStatusError
			res.Message = fmt.Sprintf("Cannot access Step Function: %v", err)
			return res, nil
		}
		messages = append(messages, "Step Function is accessible")
	}

	messages = append(messages, "S3 Bucket access is not being tested.")

	// Combine all success messages
	message := "Data source is working"
	if len(messages) > 0 {
		message = fmt.Sprintf("Data source is working: %s", strings.Join(messages, ","))
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: message,
	}, nil
}

func (d *Datasource) validateSettings(ctx context.Context) error {
	if d.settings.StepFunctionArn == "" {
		return fmt.Errorf("Step Function ARN not configured")
	}
	if d.settings.S3Bucket == "" {
		return fmt.Errorf("S3 Bucket name not configured")
	}
	return nil
}
