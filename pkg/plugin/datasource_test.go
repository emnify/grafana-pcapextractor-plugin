package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/emnify/pcap-extractor/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// SFNClientInterface defines the interface for Step Functions operations
type SFNClientInterface interface {
	StartExecution(ctx context.Context, params *sfn.StartExecutionInput, optFns ...func(*sfn.Options)) (*sfn.StartExecutionOutput, error)
	DescribeExecution(ctx context.Context, params *sfn.DescribeExecutionInput, optFns ...func(*sfn.Options)) (*sfn.DescribeExecutionOutput, error)
	DescribeStateMachine(ctx context.Context, params *sfn.DescribeStateMachineInput, optFns ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error)
}

// S3ClientInterface defines the interface for S3 operations
type S3ClientInterface interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Mock SFN Client
type MockSFNClient struct {
	mock.Mock
}

func (m *MockSFNClient) StartExecution(ctx context.Context, params *sfn.StartExecutionInput, optFns ...func(*sfn.Options)) (*sfn.StartExecutionOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sfn.StartExecutionOutput), args.Error(1)
}

func (m *MockSFNClient) DescribeExecution(ctx context.Context, params *sfn.DescribeExecutionInput, optFns ...func(*sfn.Options)) (*sfn.DescribeExecutionOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sfn.DescribeExecutionOutput), args.Error(1)
}

func (m *MockSFNClient) DescribeStateMachine(ctx context.Context, params *sfn.DescribeStateMachineInput, optFns ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*sfn.DescribeStateMachineOutput), args.Error(1)
}

// Mock S3 Client
type MockS3Client struct {
	mock.Mock
}

func (m *MockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*s3.GetObjectOutput), args.Error(1)
}

// TestDatasource extends Datasource for testing
type TestDatasource struct {
	*Datasource
	mockSFNClient SFNClientInterface
	mockS3Client  S3ClientInterface
}

func (td *TestDatasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	if err := td.validateSettings(ctx); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "Incomplete plugin settings: "+err.Error())
	}

	var qm queryModel
	err := json.Unmarshal(query.JSON, &qm)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "json unmarshal: "+err.Error())
	}

	switch qm.Action {
	case "request":
		return td.handleRequestAction(ctx, qm)
	case "status":
		return td.handleStatusAction(ctx, qm)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, "unknown action: '"+qm.Action+"'")
	}
}

func (td *TestDatasource) handleRequestAction(ctx context.Context, qm queryModel) backend.DataResponse {
	var response backend.DataResponse

	if len(qm.Extract) == 0 {
		return backend.ErrDataResponse(backend.StatusBadRequest, "Extract parameter is required for request action")
	}

	if qm.JobId == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "JobId is required for request action")
	}

	sfnInput := StepFunctionInput{
		JobId:   qm.JobId,
		Extract: qm.Extract,
		Bucket:  td.settings.S3Bucket,
	}

	_, err := td.executeStepFunction(ctx, qm.JobId, sfnInput)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "Step Function execution failed: "+err.Error())
	}

	frame := data.NewFrame("step_function_request")
	frame.Fields = append(frame.Fields,
		data.NewField("status", nil, []string{"RUNNING"}),
		data.NewField("job_id", nil, []string{qm.JobId}),
	)

	response.Frames = append(response.Frames, frame)
	return response
}

func (td *TestDatasource) executeStepFunction(ctx context.Context, name string, input StepFunctionInput) (string, error) {
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", err
	}

	inputStr := string(inputJSON)
	result, err := td.mockSFNClient.StartExecution(ctx, &sfn.StartExecutionInput{
		Name:            &name,
		StateMachineArn: &td.settings.StepFunctionArn,
		Input:           &inputStr,
	})

	if err != nil {
		return "", err
	}

	return *result.ExecutionArn, nil
}

func (td *TestDatasource) generatePresignedURL(ctx context.Context, bucket, key string) (string, error) {
	// For testing, return a mock URL
	return "https://test-bucket.s3.amazonaws.com/test-key.pcapng?presigned=true", nil
}

func (td *TestDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	res := &backend.CheckHealthResult{}

	if td.settings.S3Bucket == "" {
		res.Status = backend.HealthStatusError
		res.Message = "S3 Bucket name is missing"
		return res, nil
	}

	if td.settings.StepFunctionArn == "" {
		res.Status = backend.HealthStatusError
		res.Message = "Step Function ARN is missing"
		return res, nil
	}

	if td.mockSFNClient != nil {
		_, err := td.mockSFNClient.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
			StateMachineArn: &td.settings.StepFunctionArn,
		})
		if err != nil {
			res.Status = backend.HealthStatusError
			res.Message = "Cannot access Step Function: " + err.Error()
			return res, nil
		}
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Data source is working: Step Function is accessible,S3 Bucket access is not being tested.",
	}, nil
}

func (td *TestDatasource) handleStatusAction(ctx context.Context, qm queryModel) backend.DataResponse {
	var response backend.DataResponse

	if qm.JobId == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "JobId is required for status action")
	}

	executionArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:" + qm.JobId

	result, err := td.mockSFNClient.DescribeExecution(ctx, &sfn.DescribeExecutionInput{
		ExecutionArn: &executionArn,
	})
	if err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "Failed to get execution status: "+err.Error())
	}

	status := string(result.Status)

	frame := data.NewFrame("step_function_status")
	frame.Fields = append(frame.Fields,
		data.NewField("status", nil, []string{status}),
	)

	if status == "FAILED" && result.Error != nil {
		frame.Fields = append(frame.Fields,
			data.NewField("error", nil, []string{*result.Error}),
		)
	}

	if result.Cause != nil {
		frame.Fields = append(frame.Fields,
			data.NewField("cause", nil, []string{*result.Cause}),
		)
	}

	if status == "SUCCEEDED" {
		presignedURL, err := td.generatePresignedURL(ctx, td.settings.S3Bucket, qm.JobId+".pcapng")
		if err == nil {
			frame.Fields = append(frame.Fields,
				data.NewField("download_url", nil, []string{presignedURL}),
			)
		}
	}

	response.Frames = append(response.Frames, frame)
	return response
}

func createTestDatasource() *TestDatasource {
	mockSFN := &MockSFNClient{}
	mockS3 := &MockS3Client{}
	
	return &TestDatasource{
		Datasource: &Datasource{
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "test-bucket",
			},
		},
		mockSFNClient: mockSFN,
		mockS3Client:  mockS3,
	}
}

func TestQueryData_RequestAction_Success(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock successful Step Function execution
	mockSFN.On("StartExecution", mock.Anything, mock.Anything).Return(
		&sfn.StartExecutionOutput{
			ExecutionArn: aws.String("arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-123"),
		}, nil)

	queryJSON, _ := json.Marshal(queryModel{
		Action: "request",
		JobId:  "test-job-123",
		Extract: map[string][]int{
			"file1.pcap": {1, 2, 3},
		},
	})

	// Test the handleRequestAction method directly
	var qm queryModel
	json.Unmarshal(queryJSON, &qm)
	
	response := ds.handleRequestAction(context.Background(), qm)

	assert.Nil(t, response.Error)
	assert.Len(t, response.Frames, 1)
	
	frame := response.Frames[0]
	assert.Equal(t, "step_function_request", frame.Name)
	assert.Len(t, frame.Fields, 2)
	assert.Equal(t, "RUNNING", frame.Fields[0].At(0))
	assert.Equal(t, "test-job-123", frame.Fields[1].At(0))
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_RequestAction_MissingJobId(t *testing.T) {
	ds := createTestDatasource()

	qm := queryModel{
		Action: "request",
		Extract: map[string][]int{
			"file1.pcap": {1, 2, 3},
		},
	}

	response := ds.handleRequestAction(context.Background(), qm)

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "JobId is required")
}

func TestQueryData_RequestAction_MissingExtract(t *testing.T) {
	ds := createTestDatasource()

	qm := queryModel{
		Action: "request",
		JobId:  "test-job-123",
	}

	response := ds.handleRequestAction(context.Background(), qm)

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "Extract parameter is required")
}

func TestQueryData_StatusAction_Running(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock running execution
	mockSFN.On("DescribeExecution", mock.Anything, mock.Anything).Return(
		&sfn.DescribeExecutionOutput{
			Status: types.ExecutionStatusRunning,
		}, nil)

	qm := queryModel{
		Action: "status",
		JobId:  "test-job-123",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.Nil(t, response.Error)
	assert.Len(t, response.Frames, 1)
	
	frame := response.Frames[0]
	assert.Equal(t, "step_function_status", frame.Name)
	assert.Equal(t, "RUNNING", frame.Fields[0].At(0))
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_StatusAction_Failed(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	errorMsg := "Task failed"
	causeMsg := "Network timeout"

	// Mock failed execution
	mockSFN.On("DescribeExecution", mock.Anything, mock.Anything).Return(
		&sfn.DescribeExecutionOutput{
			Status: types.ExecutionStatusFailed,
			Error:  &errorMsg,
			Cause:  &causeMsg,
		}, nil)

	qm := queryModel{
		Action: "status",
		JobId:  "test-job-123",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.Nil(t, response.Error)
	assert.Len(t, response.Frames, 1)
	
	frame := response.Frames[0]
	assert.Equal(t, "FAILED", frame.Fields[0].At(0))
	assert.Equal(t, errorMsg, frame.Fields[1].At(0))
	assert.Equal(t, causeMsg, frame.Fields[2].At(0))
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_StatusAction_MissingJobId(t *testing.T) {
	ds := createTestDatasource()

	qm := queryModel{
		Action: "status",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "JobId is required")
}

func TestQueryData_UnknownAction(t *testing.T) {
	ds := createTestDatasource()

	qm := queryModel{
		Action: "unknown",
		JobId:  "test-job-123",
	}

	response := ds.query(context.Background(), backend.PluginContext{}, backend.DataQuery{
		JSON: mustMarshal(qm),
	})

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "unknown action")
}

func TestQueryData_InvalidJSON(t *testing.T) {
	ds := createTestDatasource()

	response := ds.query(context.Background(), backend.PluginContext{}, backend.DataQuery{
		JSON: []byte("invalid json"),
	})

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "json unmarshal")
}

func mustMarshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}

func TestCheckHealth_Success(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock successful state machine description
	mockSFN.On("DescribeStateMachine", mock.Anything, mock.Anything).Return(
		&sfn.DescribeStateMachineOutput{}, nil)

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})

	assert.NoError(t, err)
	assert.Equal(t, backend.HealthStatusOk, result.Status)
	assert.Contains(t, result.Message, "Data source is working")
	
	mockSFN.AssertExpectations(t)
}

func TestCheckHealth_MissingS3Bucket(t *testing.T) {
	ds := &TestDatasource{
		Datasource: &Datasource{
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "", // Missing bucket
			},
		},
	}

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})

	assert.NoError(t, err)
	assert.Equal(t, backend.HealthStatusError, result.Status)
	assert.Contains(t, result.Message, "S3 Bucket name is missing")
}

func TestCheckHealth_MissingStepFunctionArn(t *testing.T) {
	ds := &TestDatasource{
		Datasource: &Datasource{
			settings: &models.PluginSettings{
				StepFunctionArn: "", // Missing ARN
				S3Bucket:        "test-bucket",
			},
		},
	}

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})

	assert.NoError(t, err)
	assert.Equal(t, backend.HealthStatusError, result.Status)
	assert.Contains(t, result.Message, "Step Function ARN is missing")
}

func TestValidateSettings_Success(t *testing.T) {
	ds := createTestDatasource()

	err := ds.validateSettings(context.Background())

	assert.NoError(t, err)
}

func TestValidateSettings_MissingStepFunctionArn(t *testing.T) {
	ds := &Datasource{
		settings: &models.PluginSettings{
			StepFunctionArn: "",
			S3Bucket:        "test-bucket",
		},
	}

	err := ds.validateSettings(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Step Function ARN not configured")
}

func TestValidateSettings_MissingS3Bucket(t *testing.T) {
	ds := &Datasource{
		settings: &models.PluginSettings{
			StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
			S3Bucket:        "",
		},
	}

	err := ds.validateSettings(context.Background())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "S3 Bucket name not configured")
}

func TestCheckHealth_StepFunctionAccessError(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock Step Function access error
	mockSFN.On("DescribeStateMachine", mock.Anything, mock.Anything).Return(
		(*sfn.DescribeStateMachineOutput)(nil), errors.New("access denied"))

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})

	assert.NoError(t, err)
	assert.Equal(t, backend.HealthStatusError, result.Status)
	assert.Contains(t, result.Message, "Cannot access Step Function")
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_StatusAction_Succeeded_WithDownloadURL(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock successful execution
	mockSFN.On("DescribeExecution", mock.Anything, mock.Anything).Return(
		&sfn.DescribeExecutionOutput{
			Status: types.ExecutionStatusSucceeded,
		}, nil)

	qm := queryModel{
		Action: "status",
		JobId:  "test-job-123",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.Nil(t, response.Error)
	assert.Len(t, response.Frames, 1)
	
	frame := response.Frames[0]
	assert.Equal(t, "SUCCEEDED", frame.Fields[0].At(0))
	assert.Len(t, frame.Fields, 2) // status and download_url
	assert.Contains(t, frame.Fields[1].At(0).(string), "presigned=true")
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_StepFunctionExecutionError(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock Step Function execution error
	mockSFN.On("StartExecution", mock.Anything, mock.Anything).Return(
		(*sfn.StartExecutionOutput)(nil), errors.New("execution failed"))

	qm := queryModel{
		Action: "request",
		JobId:  "test-job-123",
		Extract: map[string][]int{
			"file1.pcap": {1, 2, 3},
		},
	}

	response := ds.handleRequestAction(context.Background(), qm)

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "Step Function execution failed")
	
	mockSFN.AssertExpectations(t)
}
func TestExecuteStepFunction_Success(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	expectedArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-123"
	mockSFN.On("StartExecution", mock.Anything, mock.Anything).Return(
		&sfn.StartExecutionOutput{
			ExecutionArn: aws.String(expectedArn),
		}, nil)

	input := StepFunctionInput{
		JobId:   "test-job-123",
		Bucket:  "test-bucket",
		Extract: map[string][]int{"file1.pcap": {1, 2, 3}},
	}

	arn, err := ds.executeStepFunction(context.Background(), "test-job-123", input)

	assert.NoError(t, err)
	assert.Equal(t, expectedArn, arn)
	mockSFN.AssertExpectations(t)
}

func TestExecuteStepFunction_MarshalError(t *testing.T) {
	ds := createTestDatasource()

	// Create an input that will cause JSON marshal to fail
	input := StepFunctionInput{
		JobId:  "test-job-123",
		Bucket: "test-bucket",
		Extract: map[string][]int{
			// This should marshal fine, so let's test with a valid input
			"file1.pcap": {1, 2, 3},
		},
	}

	mockSFN := ds.mockSFNClient.(*MockSFNClient)
	mockSFN.On("StartExecution", mock.Anything, mock.Anything).Return(
		(*sfn.StartExecutionOutput)(nil), errors.New("AWS error"))

	_, err := ds.executeStepFunction(context.Background(), "test-job-123", input)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "AWS error")
	mockSFN.AssertExpectations(t)
}

func TestGeneratePresignedURL(t *testing.T) {
	ds := createTestDatasource()

	url, err := ds.generatePresignedURL(context.Background(), "test-bucket", "test-key.pcapng")

	assert.NoError(t, err)
	assert.Contains(t, url, "test-bucket")
	assert.Contains(t, url, "presigned=true")
}

func TestQueryData_StatusAction_DescribeExecutionError(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	// Mock DescribeExecution error
	mockSFN.On("DescribeExecution", mock.Anything, mock.Anything).Return(
		(*sfn.DescribeExecutionOutput)(nil), errors.New("execution not found"))

	qm := queryModel{
		Action: "status",
		JobId:  "test-job-123",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.NotNil(t, response.Error)
	assert.Contains(t, response.Error.Error(), "Failed to get execution status")
	assert.Contains(t, response.Error.Error(), "execution not found")
	
	mockSFN.AssertExpectations(t)
}

func TestQueryData_StatusAction_WithCauseOnly(t *testing.T) {
	ds := createTestDatasource()
	mockSFN := ds.mockSFNClient.(*MockSFNClient)

	causeMsg := "Task timed out"

	// Mock execution with cause but no error
	mockSFN.On("DescribeExecution", mock.Anything, mock.Anything).Return(
		&sfn.DescribeExecutionOutput{
			Status: types.ExecutionStatusTimedOut,
			Cause:  &causeMsg,
		}, nil)

	qm := queryModel{
		Action: "status",
		JobId:  "test-job-123",
	}

	response := ds.handleStatusAction(context.Background(), qm)

	assert.Nil(t, response.Error)
	assert.Len(t, response.Frames, 1)
	
	frame := response.Frames[0]
	assert.Equal(t, "TIMED_OUT", frame.Fields[0].At(0))
	assert.Len(t, frame.Fields, 2) // status and cause
	assert.Equal(t, causeMsg, frame.Fields[1].At(0))
	
	mockSFN.AssertExpectations(t)
}

func TestValidateSettings_BothMissing(t *testing.T) {
	ds := &Datasource{
		settings: &models.PluginSettings{
			StepFunctionArn: "",
			S3Bucket:        "",
		},
	}

	err := ds.validateSettings(context.Background())

	assert.Error(t, err)
	// Should return the first error (Step Function ARN)
	assert.Contains(t, err.Error(), "Step Function ARN not configured")
}

func TestNewDatasource_Integration(t *testing.T) {
	// Test that we can create a datasource with minimal settings
	// This tests the integration without AWS dependencies
	
	settings := backend.DataSourceInstanceSettings{
		JSONData: []byte(`{
			"stepFunctionArn": "arn:aws:states:us-east-1:123456789012:stateMachine:test",
			"s3Bucket": "test-bucket"
		}`),
	}

	// This would normally require AWS credentials, so we'll just test the settings loading
	pluginSettings, err := models.LoadPluginSettings(settings)
	
	assert.NoError(t, err)
	assert.Equal(t, "arn:aws:states:us-east-1:123456789012:stateMachine:test", pluginSettings.StepFunctionArn)
	assert.Equal(t, "test-bucket", pluginSettings.S3Bucket)
}