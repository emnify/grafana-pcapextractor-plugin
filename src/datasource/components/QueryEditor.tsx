import React, { ChangeEvent } from 'react';
import { Alert, InlineField, TextArea, Button, Input, Select } from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { DataSourceOptions, MyQuery } from '../types';

type Props = QueryEditorProps<DataSource, MyQuery, DataSourceOptions>;

const actionOptions: Array<SelectableValue<string>> = [
  { label: 'Request - Trigger Step Function', value: 'request' },
  { label: 'Status - Check Execution Status', value: 'status' },
];

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const onActionChange = (option: SelectableValue<string>) => {
    onChange({ ...query, action: option.value as 'request' | 'status' });
  };

  const onJobIdChange = (event: ChangeEvent<HTMLInputElement>) => {
    const JobId = event.target.value;
    const Extract = query.seriesData || {};
    onChange({ ...query, JobId, Extract });
  };

  const onBucketChange = (event: ChangeEvent<HTMLInputElement>) => {
    const Bucket = event.target.value;
    const Extract = query.seriesData || {};
    onChange({ ...query, Bucket, Extract });
  };

  const onExecutionArnChange = (event: ChangeEvent<HTMLInputElement>) => {
    const executionArn = event.target.value;
    onChange({ ...query, executionArn });
  };

  const onSeriesDataChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    try {
      const seriesData = JSON.parse(event.target.value);
      const Extract = seriesData;
      onChange({ ...query, seriesData, Extract });
    } catch (e) {
      // Invalid JSON, don't update the query
    }
  };

  const onRunQueryClick = () => {
    onRunQuery();
  };

  const isRequestAction = query.action === 'request' || !query.action;
  const isStatusAction = query.action === 'status';

  return (
    <div>
      <Alert title="PCAP Extractor Data Source" severity="info">
        This data source can trigger AWS Step Functions or check their execution status.
      </Alert>
      
      <InlineField label="Action" labelWidth={20} tooltip="Choose the action to perform">
        <Select
          options={actionOptions}
          value={actionOptions.find(option => option.value === (query.action || 'request'))}
          onChange={onActionChange}
          width={40}
        />
      </InlineField>

      {isRequestAction && (
        <>
          <InlineField label="Job ID" labelWidth={20} tooltip="Unique identifier for the PCAP extraction job">
            <Input
              placeholder="job-12345"
              value={query.JobId || ''}
              onChange={onJobIdChange}
              width={40}
            />
          </InlineField>
          
          <InlineField label="S3 Bucket" labelWidth={20} tooltip="S3 bucket name where the PCAP file will be stored">
            <Input
              placeholder="my-pcap-bucket"
              value={query.Bucket || ''}
              onChange={onBucketChange}
              width={40}
            />
          </InlineField>
          
          <InlineField label="Extract Data" labelWidth={20} tooltip="JSON object with S3 paths as keys and arrays of numbers as values">
            <TextArea
              placeholder='{"s3://foo/bar.pcapng": [1, 42], "s3://foo/bar2.pcapng": [45]}'
              value={JSON.stringify(query.seriesData || {}, null, 2)}
              onChange={onSeriesDataChange}
              rows={6}
              cols={80}
            />
          </InlineField>
        </>
      )}

      {isStatusAction && (
        <>
          <InlineField label="Execution ARN" labelWidth={20} tooltip="Step Function execution ARN to check status">
            <Input
              placeholder="arn:aws:states:region:account:execution:stateMachine:executionName"
              value={query.executionArn || ''}
              onChange={onExecutionArnChange}
              width={80}
            />
          </InlineField>
          
          <InlineField label="Job ID" labelWidth={20} tooltip="Job ID (optional, for generating presigned URL if execution succeeded)">
            <Input
              placeholder="job-12345"
              value={query.JobId || ''}
              onChange={onJobIdChange}
              width={40}
            />
          </InlineField>
          
          <InlineField label="S3 Bucket" labelWidth={20} tooltip="S3 bucket (optional, for generating presigned URL if execution succeeded)">
            <Input
              placeholder="my-pcap-bucket"
              value={query.Bucket || ''}
              onChange={onBucketChange}
              width={40}
            />
          </InlineField>
        </>
      )}
      
      <Button onClick={onRunQueryClick} variant="primary">
        {isRequestAction ? 'Execute Step Function' : 'Check Status'}
      </Button>
    </div>
  );
}
