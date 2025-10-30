import React, { ChangeEvent } from 'react';
import {Divider, InlineField, Input,} from '@grafana/ui';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { DataSourceOptions } from '../types';
import { ConnectionConfig} from '@grafana/aws-sdk';
import { ConfigSection } from '@grafana/plugin-ui';

interface Props extends DataSourcePluginOptionsEditorProps<DataSourceOptions> { }

export function ConfigEditor(props: Props) {
  const { options, onOptionsChange } = props;
  const { jsonData } = options;

  const onStepFunctionArnChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        stepFunctionArn: event.target.value,
      },
    });
  };

  const onS3BucketChange = (event: ChangeEvent<HTMLInputElement>) => {
    onOptionsChange({
      ...options,
      jsonData: {
        ...jsonData,
        s3Bucket: event.target.value,
      },
    });
  };

  return (
    <>
      <ConnectionConfig {...props} />

      <Divider />
      <ConfigSection title="PCAP Exporter">

          <InlineField label="Step Function ARN" labelWidth={20} interactive tooltip={'AWS Step Function ARN to execute'}>
            <Input
              id="config-editor-stepfunction-arn"
              onChange={onStepFunctionArnChange}
              value={jsonData.stepFunctionArn || ''}
              placeholder="arn:aws:states:region:account-id:stateMachine:stateMachineName"
              width={60}
            />
          </InlineField>

          <InlineField label="S3 Bucket" labelWidth={20} interactive tooltip={'S3 bucket name for data storage'}>
            <Input
              id="config-editor-s3-bucket"
              onChange={onS3BucketChange}
              value={jsonData.s3Bucket || ''}
              placeholder="$stage-$instance-pcap-extractor"
              width={60}
            />
          </InlineField>

      </ConfigSection>
    </>
  );
}
