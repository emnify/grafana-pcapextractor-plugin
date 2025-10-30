import { DataQuery } from '@grafana/schema';
import { AwsAuthDataSourceJsonData } from '@grafana/aws-sdk';

export interface MyQuery extends DataQuery {
  queryText?: string;
  constant: number;
  dashboardUid?: string; // Dashboard UID to read data from (per query)
  panelId?: number; // Panel ID to read data from (per query)
  seriesData?: { [key: string]: number[] }; // Series data for Step Function execution
  JobId?: string; // Job ID for the PCAP extraction
  Bucket?: string; // S3 bucket name
  Extract?: { [key: string]: number[] }; // Extract data (same as seriesData but with proper naming)
  action?: 'request' | 'status'; // Query action type
  executionArn?: string; // Step Function execution ARN for status queries
}

export const DEFAULT_QUERY: Partial<MyQuery> = {
  constant: 6.5,
  seriesData: {},
  JobId: '',
  Bucket: '',
  Extract: {},
  action: 'request',
};

export interface DataPoint {
  Time: number;
  Value: number;
}

export interface DataSourceResponse {
  datapoints: DataPoint[];
}

export interface DataSourceOptions extends AwsAuthDataSourceJsonData {
  stepFunctionArn?: string;
  s3Bucket?: string;
}
