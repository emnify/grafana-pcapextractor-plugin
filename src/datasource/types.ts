import { DataQuery } from '@grafana/schema';
import { AwsAuthDataSourceJsonData } from '@grafana/aws-sdk';

export interface Query extends DataQuery {
  bucket: string; // Job ID for the PCAP extraction
  action: 'request' | 'status';
  extract?: { [key: string]: number[] };
}

export interface DataSourceOptions extends AwsAuthDataSourceJsonData {
  stepFunctionArn?: string;
  s3Bucket?: string;
}
