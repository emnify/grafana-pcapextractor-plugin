import { E2ESelectors } from '@grafana/e2e-selectors';

export const Components = {
  ConfigEditor: {
    StepFunctionArn: {
      input: 'Step Function ARN',
      testID: 'data-testid stepFunctionArn',
    },
    S3bucket: {
      input: 'S3 Bucket Name',
      testID: 'data-testid s3BucketName',
    }
  },
  QueryEditor: {
    CodeEditor: {
      container: 'Code editor container',
    },
    TableView: {
      input: 'toggle-table-view',
    },
  },
  RefreshPicker: {
    runButton: 'RefreshPicker run button',
  },
};

export const selectors: { components: E2ESelectors<typeof Components> } = {
  components: Components,
};
