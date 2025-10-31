package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/emnify/pcap-extractor/pkg/models"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSFNClient is a mock implementation of the Step Functions client
type MockSFNClient struct {
	mock.Mock
}

// MockS3Presigner is a mock implementation of the S3 presigner
type MockS3Presigner struct {
	mock.Mock
}

func (m *MockSFNClient) StartExecution(ctx context.Context, params *sfn.StartExecutionInput, optFns ...func(*sfn.Options)) (*sfn.StartExecutionOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sfn.StartExecutionOutput), args.Error(1)
}

func (m *MockSFNClient) DescribeExecution(ctx context.Context, params *sfn.DescribeExecutionInput, optFns ...func(*sfn.Options)) (*sfn.DescribeExecutionOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sfn.DescribeExecutionOutput), args.Error(1)
}

func (m *MockSFNClient) DescribeStateMachine(ctx context.Context, params *sfn.DescribeStateMachineInput, optFns ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sfn.DescribeStateMachineOutput), args.Error(1)
}

func (m *MockS3Presigner) PresignGetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*v4.PresignedHTTPRequest), args.Error(1)
}

func TestHandleRequestAction(t *testing.T) {
	tests := []struct {
		name           string
		queryModel     queryModel
		setupMock      func(*MockSFNClient)
		expectedStatus backend.Status
		expectedError  string
		validateFrame  func(*testing.T, backend.DataResponse)
	}{
		{
			name: "successful request with valid data",
			queryModel: queryModel{
				Action: "request",
				JobId:  "test-job-123",
				Extract: map[string][]int{
					"file1.pcap": {1, 2, 3},
					"file2.pcap": {4, 5, 6},
				},
			},
			setupMock: func(mockClient *MockSFNClient) {
				executionArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-123"
				mockClient.On("StartExecution", mock.Anything, mock.MatchedBy(func(input *sfn.StartExecutionInput) bool {
					return *input.Name == "test-job-123" && 
						   *input.StateMachineArn == "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine"
				})).Return(&sfn.StartExecutionOutput{
					ExecutionArn: &executionArn,
				}, nil)
			},
			expectedStatus: backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_request", frame.Name)
				assert.Len(t, frame.Fields, 2)
				
				// Check status field
				statusField := frame.Fields[0]
				assert.Equal(t, "status", statusField.Name)
				statusValues, ok := statusField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "RUNNING", statusValues)
				
				// Check job_id field
				jobIdField := frame.Fields[1]
				assert.Equal(t, "job_id", jobIdField.Name)
				jobIdValues, ok := jobIdField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "test-job-123", jobIdValues)
			},
		},
		{
			name: "missing extract parameter",
			queryModel: queryModel{
				Action:  "request",
				JobId:   "test-job-123",
				Extract: map[string][]int{},
			},
			setupMock:      func(mockClient *MockSFNClient) {},
			expectedStatus: backend.StatusBadRequest,
			expectedError:  "Extract parameter is required for request action",
		},
		{
			name: "missing job ID",
			queryModel: queryModel{
				Action: "request",
				JobId:  "",
				Extract: map[string][]int{
					"file1.pcap": {1, 2, 3},
				},
			},
			setupMock:      func(mockClient *MockSFNClient) {},
			expectedStatus: backend.StatusBadRequest,
			expectedError:  "JobId is required for request action",
		},
		{
			name: "step function execution failure",
			queryModel: queryModel{
				Action: "request",
				JobId:  "test-job-123",
				Extract: map[string][]int{
					"file1.pcap": {1, 2, 3},
				},
			},
			setupMock: func(mockClient *MockSFNClient) {
				mockClient.On("StartExecution", mock.Anything, mock.Anything).Return(
					(*sfn.StartExecutionOutput)(nil), 
					errors.New("execution failed"),
				)
			},
			expectedStatus: backend.StatusBadRequest,
			expectedError:  "Step Function execution failed:",
		},
		{
			name: "validates step function input structure",
			queryModel: queryModel{
				Action: "request",
				JobId:  "test-job-456",
				Extract: map[string][]int{
					"capture1.pcap": {10, 20, 30},
				},
			},
			setupMock: func(mockClient *MockSFNClient) {
				executionArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-456"
				mockClient.On("StartExecution", mock.Anything, mock.MatchedBy(func(input *sfn.StartExecutionInput) bool {
					// Validate the input contains the expected JSON structure
					if input.Input == nil {
						return false
					}
					inputStr := *input.Input
					return *input.Name == "test-job-456" && 
						   *input.StateMachineArn == "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine" &&
						   len(inputStr) > 0 // Basic validation that input is not empty
				})).Return(&sfn.StartExecutionOutput{
					ExecutionArn: &executionArn,
				}, nil)
			},
			expectedStatus: backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_request", frame.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock SFN client
			mockSFNClient := &MockSFNClient{}
			tt.setupMock(mockSFNClient)

			// Create datasource with mock client
			ds := &Datasource{
				settings: &models.PluginSettings{
					StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
					S3Bucket:        "test-bucket",
				},
				sfnClient: mockSFNClient,
			}

			// Execute the function
			ctx := context.Background()
			response := ds.handleRequestAction(ctx, tt.queryModel)

			// Validate response status
			if tt.expectedStatus == backend.StatusOK {
				assert.Empty(t, response.Error)
				if tt.validateFrame != nil {
					tt.validateFrame(t, response)
				}
			} else {
				assert.NotNil(t, response.Error)
				assert.Contains(t, response.Error.Error(), tt.expectedError)
			}

			// Verify all mock expectations were met
			mockSFNClient.AssertExpectations(t)
		})
	}
}

func TestHandleStatusAction(t *testing.T) {
	tests := []struct {
		name             string
		queryModel       queryModel
		setupSFNMock     func(*MockSFNClient)
		setupS3Mock      func(*MockS3Presigner)
		expectedStatus   backend.Status
		expectedError    string
		validateFrame    func(*testing.T, backend.DataResponse)
		needsS3Presigner bool
	}{
		{
			name: "successful status check - running execution",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-123",
			},
			setupSFNMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeExecution", mock.Anything, mock.MatchedBy(func(input *sfn.DescribeExecutionInput) bool {
					expectedArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-123"
					return *input.ExecutionArn == expectedArn
				})).Return(&sfn.DescribeExecutionOutput{
					Status: "RUNNING",
				}, nil)
			},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_status", frame.Name)
				assert.Len(t, frame.Fields, 1)
				
				// Check status field
				statusField := frame.Fields[0]
				assert.Equal(t, "status", statusField.Name)
				statusValue, ok := statusField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "RUNNING", statusValue)
			},
		},
		{
			name: "successful status check - completed execution",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-456",
			},
			setupSFNMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeExecution", mock.Anything, mock.MatchedBy(func(input *sfn.DescribeExecutionInput) bool {
					expectedArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-456"
					return *input.ExecutionArn == expectedArn
				})).Return(&sfn.DescribeExecutionOutput{
					Status: "COMPLETED",
				}, nil)
			},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_status", frame.Name)
				assert.Len(t, frame.Fields, 1)
				
				// Check status field
				statusField := frame.Fields[0]
				assert.Equal(t, "status", statusField.Name)
				statusValue, ok := statusField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "COMPLETED", statusValue)
			},
		},
		{
			name: "successful status check - succeeded execution with presigned URL",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-succeeded",
			},
			setupSFNMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeExecution", mock.Anything, mock.MatchedBy(func(input *sfn.DescribeExecutionInput) bool {
					expectedArn := "arn:aws:states:us-east-1:123456789012:execution:test-state-machine:test-job-succeeded"
					return *input.ExecutionArn == expectedArn
				})).Return(&sfn.DescribeExecutionOutput{
					Status: "SUCCEEDED",
				}, nil)
			},
			setupS3Mock: func(mockPresigner *MockS3Presigner) {
				mockPresigner.On("PresignGetObject", mock.Anything, mock.MatchedBy(func(input *s3.GetObjectInput) bool {
					return *input.Bucket == "test-bucket" && *input.Key == "test-job-succeeded.pcapng"
				})).Return(&v4.PresignedHTTPRequest{
					URL: "https://test-bucket.s3.amazonaws.com/test-job-succeeded.pcapng?presigned=true",
				}, nil)
			},
			needsS3Presigner: true,
			expectedStatus: backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_status", frame.Name)
				assert.Len(t, frame.Fields, 2) // status and download_url
				
				// Check status field
				statusField := frame.Fields[0]
				assert.Equal(t, "status", statusField.Name)
				statusValue, ok := statusField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "SUCCEEDED", statusValue)
				
				// Check download_url field
				downloadField := frame.Fields[1]
				assert.Equal(t, "download_url", downloadField.Name)
				downloadValue, ok := downloadField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "https://test-bucket.s3.amazonaws.com/test-job-succeeded.pcapng?presigned=true", downloadValue)
			},
		},
		{
			name: "failed execution with error and cause",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-failed",
			},
			setupSFNMock: func(mockClient *MockSFNClient) {
				errorMsg := "Task failed"
				causeMsg := "Network timeout"
				mockClient.On("DescribeExecution", mock.Anything, mock.Anything).Return(&sfn.DescribeExecutionOutput{
					Status: "FAILED",
					Error:  &errorMsg,
					Cause:  &causeMsg,
				}, nil)
			},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusOK,
			validateFrame: func(t *testing.T, response backend.DataResponse) {
				assert.Len(t, response.Frames, 1)
				frame := response.Frames[0]
				assert.Equal(t, "step_function_status", frame.Name)
				assert.Len(t, frame.Fields, 3) // status, error, cause
				
				// Check status field
				statusField := frame.Fields[0]
				assert.Equal(t, "status", statusField.Name)
				statusValue, ok := statusField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "FAILED", statusValue)
				
				// Check error field
				errorField := frame.Fields[1]
				assert.Equal(t, "error", errorField.Name)
				errorValue, ok := errorField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "Task failed", errorValue)
				
				// Check cause field
				causeField := frame.Fields[2]
				assert.Equal(t, "cause", causeField.Name)
				causeValue, ok := causeField.At(0).(string)
				assert.True(t, ok)
				assert.Equal(t, "Network timeout", causeValue)
			},
		},
		{
			name: "missing job ID",
			queryModel: queryModel{
				Action: "status",
				JobId:  "",
			},
			setupSFNMock:     func(mockClient *MockSFNClient) {},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusBadRequest,
			expectedError:    "JobId is required for status action",
		},
		{
			name: "invalid step function ARN",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-123",
			},
			setupSFNMock:     func(mockClient *MockSFNClient) {},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusBadRequest,
			expectedError:    "Failed to parse Step Function ARN",
		},
		{
			name: "describe execution failure",
			queryModel: queryModel{
				Action: "status",
				JobId:  "test-job-123",
			},
			setupSFNMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeExecution", mock.Anything, mock.Anything).Return(
					(*sfn.DescribeExecutionOutput)(nil),
					errors.New("execution not found"),
				)
			},
			setupS3Mock:      func(mockPresigner *MockS3Presigner) {},
			needsS3Presigner: false,
			expectedStatus:   backend.StatusBadRequest,
			expectedError:    "Failed to get execution status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock SFN client
			mockSFNClient := &MockSFNClient{}
			tt.setupSFNMock(mockSFNClient)

			// Create mock S3 presigner if needed
			var mockS3Presigner *MockS3Presigner
			if tt.needsS3Presigner {
				mockS3Presigner = &MockS3Presigner{}
				tt.setupS3Mock(mockS3Presigner)
			}

			// Create datasource with mock clients
			stepFunctionArn := "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine"
			if tt.name == "invalid step function ARN" {
				stepFunctionArn = "invalid-arn"
			}

			ds := &Datasource{
				settings: &models.PluginSettings{
					StepFunctionArn: stepFunctionArn,
					S3Bucket:        "test-bucket",
				},
				sfnClient:   mockSFNClient,
				s3Presigner: mockS3Presigner,
			}

			// Execute the function
			ctx := context.Background()
			response := ds.handleStatusAction(ctx, tt.queryModel)

			// Validate response status
			if tt.expectedStatus == backend.StatusOK {
				assert.Empty(t, response.Error)
				if tt.validateFrame != nil {
					tt.validateFrame(t, response)
				}
			} else {
				assert.NotNil(t, response.Error)
				assert.Contains(t, response.Error.Error(), tt.expectedError)
			}

			// Verify all mock expectations were met
			mockSFNClient.AssertExpectations(t)
			if mockS3Presigner != nil {
				mockS3Presigner.AssertExpectations(t)
			}
		})
	}
}

func TestCheckHealth(t *testing.T) {
	tests := []struct {
		name           string
		settings       *models.PluginSettings
		setupMock      func(*MockSFNClient)
		expectedStatus backend.HealthStatus
		expectedMsg    string
	}{
		{
			name: "successful health check",
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "test-bucket",
			},
			setupMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeStateMachine", mock.Anything, mock.MatchedBy(func(input *sfn.DescribeStateMachineInput) bool {
					return *input.StateMachineArn == "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine"
				})).Return(&sfn.DescribeStateMachineOutput{}, nil)
			},
			expectedStatus: backend.HealthStatusOk,
			expectedMsg:    "Data source is working: Step Function is accessible,S3 Bucket access is not being tested.",
		},
		{
			name: "missing S3 bucket configuration",
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "",
			},
			setupMock:      func(mockClient *MockSFNClient) {},
			expectedStatus: backend.HealthStatusError,
			expectedMsg:    "S3 Bucket name is missing",
		},
		{
			name: "missing step function ARN configuration",
			settings: &models.PluginSettings{
				StepFunctionArn: "",
				S3Bucket:        "test-bucket",
			},
			setupMock:      func(mockClient *MockSFNClient) {},
			expectedStatus: backend.HealthStatusError,
			expectedMsg:    "Step Function ARN is missing",
		},
		{
			name: "step function access denied",
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "test-bucket",
			},
			setupMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeStateMachine", mock.Anything, mock.Anything).Return(
					(*sfn.DescribeStateMachineOutput)(nil),
					errors.New("AccessDenied: User is not authorized to perform: states:DescribeStateMachine"),
				)
			},
			expectedStatus: backend.HealthStatusError,
			expectedMsg:    "Cannot access Step Function: AccessDenied: User is not authorized to perform: states:DescribeStateMachine",
		},
		{
			name: "step function not found",
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:nonexistent-state-machine",
				S3Bucket:        "test-bucket",
			},
			setupMock: func(mockClient *MockSFNClient) {
				mockClient.On("DescribeStateMachine", mock.Anything, mock.Anything).Return(
					(*sfn.DescribeStateMachineOutput)(nil),
					errors.New("StateMachineDoesNotExist: State Machine Does Not Exist"),
				)
			},
			expectedStatus: backend.HealthStatusError,
			expectedMsg:    "Cannot access Step Function: StateMachineDoesNotExist: State Machine Does Not Exist",
		},
		{
			name: "nil sfn client",
			settings: &models.PluginSettings{
				StepFunctionArn: "arn:aws:states:us-east-1:123456789012:stateMachine:test-state-machine",
				S3Bucket:        "test-bucket",
			},
			setupMock:      nil, // This will result in nil sfnClient
			expectedStatus: backend.HealthStatusOk,
			expectedMsg:    "Data source is working: S3 Bucket access is not being tested.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create datasource
			ds := &Datasource{
				settings: tt.settings,
			}

			// Setup mock SFN client if needed
			if tt.setupMock != nil {
				mockSFNClient := &MockSFNClient{}
				tt.setupMock(mockSFNClient)
				ds.sfnClient = mockSFNClient
				defer mockSFNClient.AssertExpectations(t)
			}

			// Execute CheckHealth
			ctx := context.Background()
			req := &backend.CheckHealthRequest{}
			result, err := ds.CheckHealth(ctx, req)

			// Validate results
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedMsg, result.Message)
		})
	}
}