export interface PcapExtractorOptions {
  pcapExtractorDataSource?: string;
  text: string;
}

export type QueryTemplate = {
  refId: string,
  datasource: {
    type: string,
    uid: string,
  },
  jobId: string;
  action: 'request' | 'status';
  extract?: { [key: string]: number[] };
}
