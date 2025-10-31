import React, { useState, useRef, useCallback } from 'react';
import { PanelProps, Field, QueryResultMeta } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { PcapExtractorOptions, QueryTemplate } from 'panel/types';
import { css } from '@emotion/css';
import { useStyles2, Button, Alert, Spinner } from '@grafana/ui';

interface Props extends PanelProps<PcapExtractorOptions> { }

const getStyles = () => {
  return {
    buttonContainer: css`
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
    `,
    infoText: css`
      font-size: 12px;
      color: #6c757d;
      text-align: center;
    `,
    errorContainer: css`
      width: 100%;
      max-width: 600px;
      margin-top: 8px;
    `,
    spinner: css`
      margin-right: 8px;
    `
  };
};

const parseResponse = (response: any): Map<string, string> => {
  const fieldValues = new Map<string, string>();

  if (!response?.results?.['pcap-extract']) {
    return fieldValues;
  }

  const pcapResult = response.results['pcap-extract'];

  if (!pcapResult.frames?.[0]) {
    return fieldValues;
  }

  const frame = pcapResult.frames[0];

  if (!frame?.data?.values || !frame?.schema?.fields) {
    return fieldValues;
  }

  frame.schema.fields.forEach((field: any, index: number) => {
    if (field.name && frame.data.values[index] && frame.data.values[index].length > 0) {
      fieldValues.set(field.name, frame.data.values[index][0]);
    }
  });

  return fieldValues;
};


const getQueryTemplate = (action: 'request' | 'status', jobId: string, options: any): QueryTemplate => {
  const query: QueryTemplate = {
    refId: 'pcap-extract',
    datasource: {
      type: 'emnify-pcapextractor-datasource',
      uid: options.pcapExtractorDataSource
    },
    action: action,
    jobId: jobId
  };

  return query;
}

async function queryBackend(query: QueryTemplate) {
  const requestPayload = {
    queries: [query],
  };
  const backendSrv = getBackendSrv();

  const apiUrl = '/api/ds/query';

  window.console.log('Sending request to backend', requestPayload);
  const response = await backendSrv.post(apiUrl, requestPayload);
  window.console.log('Backend response received', response);

  return parseResponse(response);
}

const transformExtractData = (seriesData: {
  name?: string | undefined;
  fields: Field[] | undefined;
  length: number | undefined;
  refId?: string | undefined;
  meta?: QueryResultMeta | undefined
})=>  {

  // Extract only the required fields: source_file and source_packet_number
  const requiredFieldNames = ['source_file', 'source_packet_number'];
  const extractedFields = seriesData?.fields?.filter(field =>
    requiredFieldNames.includes(field.name)
  ).map(field => ({
    name: field.name,
    type: field.type,
    values: field.values ? Array.from(field.values) : []
  })) || [];

  // Validate that all required fields are present
  const missingFields = requiredFieldNames.filter(required => 
    !extractedFields.map(field => field.name).includes(required)
  );

  if (missingFields.length > 0) {
    throw new Error(`Missing required fields: ${missingFields.join(', ')}`)
  }

  // Transform data to group packet numbers by source file
  const sourceFileField = extractedFields.find(field => field.name === 'source_file');
  const packetNumberField = extractedFields.find(field => field.name === 'source_packet_number');

  const extractData: { [sourceFile: string]: number[] } = {};

  if (sourceFileField && packetNumberField) {
    const sourceFiles = sourceFileField.values;
    const packetNumbers = packetNumberField.values;

    for (let i = 0; i < sourceFiles.length; i++) {
      const sourceFile = sourceFiles[i] as string;
      const packetNumber = packetNumbers[i] as number;

      if (!extractData[sourceFile]) {
        extractData[sourceFile] = [];
      }

      if (!extractData[sourceFile].includes(packetNumber)) {
        extractData[sourceFile].push(packetNumber);
      }
    }
  }

  return extractData;
};

function stopPolling(pollingIntervalRef: React.MutableRefObject<NodeJS.Timeout | null>) {
  // Clear polling interval
  if (pollingIntervalRef.current) {
    clearInterval(pollingIntervalRef.current);
    pollingIntervalRef.current = null;
  }
}

function triggerDownload(downloadUrl: string | undefined) {
  // Trigger browser download
  if (downloadUrl) {
    const link = document.createElement('a');
    link.href = downloadUrl;
    link.download = ''; // Let the browser determine filename from URL/headers
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    window.console.log('Download triggered for URL:', downloadUrl);
  }
}

export const Download: React.FC<Props> = ({ options, data }) => {

  const styles = useStyles2(getStyles);
  const [downloadState, setDownloadState] = useState<'idle' | 'processing' | 'downloaded' | 'error'>('idle');
  const [error, setError] = useState<string | null>(null);
  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);

  // Log component initialization and prop changes
  // FIXME remove unless considered useful
  React.useEffect(() => {
    window.console.log('=== Download Component Initialized/Updated ===');
    window.console.log('Panel data summary:', {
      seriesCount: data.series?.length || 0,
      timeRange: data.timeRange,
      state: data.state
    });
  }, [options, data]);


  // Clear polling interval when component unmounts or when polling stops
  React.useEffect(() => {
    return () => {
      stopPolling(pollingIntervalRef)
    };
  }, []);

  const pollJobStatus = useCallback(async (jobId: string) => {
    try {
      window.console.log(`Polling job status for runId: ${jobId}`);

      const query = getQueryTemplate('status', jobId, options);
      const response = await queryBackend(query);

      const status = response.get('status') || null;

      if (status) {
        if (status === 'RUNNING') {
          window.console.log('Job still running, continuing to poll...');
        } else if (status === 'SUCCEEDED') {
          window.console.log('Job completed successfully!');

          const downloadUrl = response.get('download_url');
          window.console.log('Download URL:', downloadUrl);

          setDownloadState('downloaded');
          triggerDownload(downloadUrl);
          stopPolling(pollingIntervalRef);

        } else {
          window.console.error('Job ended with status:', status);
          let errorMessage = `Job failed with status: ${status}`;
          if (response.has('error')) {
            errorMessage += `\nError: ${response.get('error')}`;
          }
          if (response.has('cause')) {
            errorMessage += `\nCause: ${response.get('cause')}`;
          }
          setError(errorMessage);
          stopPolling(pollingIntervalRef);
        }
      } else {
        setError('Invalid status response format - no status found in response:' + response);
      }
    } catch (error) {
      window.console.error('Error polling job status:', error);
      let errorMessage = JSON.stringify(error, null, 2);
      setError(`Failed to poll status: ${errorMessage}`);
      stopPolling(pollingIntervalRef)
    }
  }, [options]);

  const handleDownload = async () => {

    // FIXME make this dependent on the data so that we don't process the
    // same file+packet extraction twice
    const jobId = "run-" + Date.now();
    setDownloadState('processing');
    setError(null);

    try {
      window.console.log('⬇️ Download started requested');

      if (!options.pcapExtractorDataSource) {
        throw new Error('PCAP Extractor data source not configured');
      }

      const seriesData = data.series?.[0];
      let extractData;
      try {
        extractData = transformExtractData(seriesData);
      } catch (error){
        setError("" + error)
        return;
      }

      window.console.log('Extract data prepared', extractData);

      let query = getQueryTemplate('request', jobId, options);
      query.extract = extractData

      const response = await queryBackend(query)
      window.console.log('Received response data', response);

      window.console.log('✓ Download request submitted, now polling for status');

      // Start polling for step function status
      pollingIntervalRef.current = setInterval(() => {
        pollJobStatus(jobId);
      }, 10000); // Poll every 10 seconds

    } catch (error) {
      window.console.error('❌ PCAP Download Process Failed');
      window.console.error('Error details:', error);

      let errorMessage = JSON.stringify(error, null, 2);
      setError(`Failed to request PCAP extraction: ${errorMessage}`);
    }
  };

  const isConfigured = options.pcapExtractorDataSource;
  const hasData = data.series && data.series.length > 0;
  const isDataLoading = data.state === 'Loading';

  // Reset download state when data starts loading or when data changes
  React.useEffect(() => {
    if (isDataLoading && (downloadState === 'downloaded' || downloadState === 'error')) {
      setDownloadState('idle');
      setError(null);

      stopPolling(pollingIntervalRef)
    }
  }, [isDataLoading, downloadState]);

  const getButtonProps = () => {
    switch (downloadState) {
      case 'processing':
        return {
          text: 'Processing',
          icon: undefined,
          variant: 'primary' as const,
          disabled: true
        };
      case 'downloaded':
        return {
          text: 'Download Finished',
          icon: 'check' as const,
          variant: 'success' as const,
        };
      default:
        return {
          text: options.text || 'Download PCAP',
          icon: 'download-alt' as const,
          variant: 'primary' as const,
          disabled: !isConfigured || !hasData || isDataLoading
        };
    }
  };

  const buttonProps = getButtonProps();

  return (
    <div>
      <div className={styles.buttonContainer}>
        <Button
          variant={buttonProps.variant}
          size="md"
          icon={buttonProps.icon}
          onClick={handleDownload}
          disabled={buttonProps.disabled}
        >
          {downloadState === 'processing' && <Spinner className={styles.spinner} size="sm" />}
          {buttonProps.text}
        </Button>

        {!isConfigured && (
          <div className={styles.infoText}>
            Configure PCAP Extractor data source in panel options
          </div>
        )}

        {isConfigured && !hasData && !isDataLoading && (
          <div className={styles.infoText}>
            No series data available to send
          </div>
        )}

        {isConfigured && isDataLoading && (
          <div className={styles.infoText}>
            Loading panel data...
          </div>
        )}

      </div>

      {error && (
        <div className={styles.errorContainer}>
          <Alert title="Download Failed" severity="error">
            <div style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: '13px', lineHeight: '1.4' }}>
              {error}
            </div>
            {downloadState === 'error' && (
              <div style={{ marginTop: '8px', fontSize: '12px', color: '#666' }}>
                You can try downloading again or check the panel configuration.
              </div>
            )}
          </Alert>
        </div>
      )}
    </div>
  );
};
