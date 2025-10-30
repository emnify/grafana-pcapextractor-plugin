import React, { useState, useRef, useCallback } from 'react';
import { PanelProps } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { PcapExtractorOptions } from 'types';
import { css, cx } from '@emotion/css';
import { useStyles2, Button, Alert, Spinner } from '@grafana/ui';

interface Props extends PanelProps<PcapExtractorOptions> { }

const getStyles = () => {
  return {
    wrapper: css`
      //font-family: Open Sans;
      position: relative;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      gap: 16px;
      padding: 16px;
    `,
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
  };
};

// Helper function to extract field values from response
const extractFieldValues = (response: any): Map<string, string> => {
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

export const Download: React.FC<Props> = ({ options, data, width, height }) => {

  const styles = useStyles2(getStyles);
  const [downloadState, setDownloadState] = useState<'idle' | 'processing' | 'downloaded' | 'error'>('idle');
  const [error, setError] = useState<string | null>(null);
  // const [jobId, setJobId] = useState<string | null>(null);
  const pollingIntervalRef = useRef<NodeJS.Timeout | null>(null);

  // Log component initialization and prop changes
  React.useEffect(() => {
    window.console.log('=== Download Component Initialized/Updated ===');
    window.console.log('Panel options:', options);
    window.console.log('Panel data summary:', {
      seriesCount: data.series?.length || 0,
      timeRange: data.timeRange,
      state: data.state
    });
    window.console.log('Component dimensions:', { width, height });
  }, [options, data, width, height]);


  // Clear polling interval when component unmounts or when polling stops
  React.useEffect(() => {
    return () => {
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
      }
    };
  }, []);

  // Function to poll for job status
  const pollJobStatus = useCallback(async (jobId: string) => {
    try {
      window.console.log(`Polling job status for runId: ${jobId}`);

      const backendSrv = getBackendSrv();

      const query = {
        refId: 'pcap-extract',
        datasource: {
          type: 'emnify-pcapextractor-datasource',
          uid: options.pcapExtractorDataSource
        },
        action: 'status',
        jobId: jobId,
        timeRange: data.timeRange,

      };

      const requestPayload = {
        queries: [query],
        from: data.timeRange.from.valueOf().toString(),
        to: data.timeRange.to.valueOf().toString(),
      };

      const apiUrl = `/api/ds/query?ds_type=emnify-pcapdownload-datasource&requestId=${Date.now()}`;
      const response = await backendSrv.post(apiUrl, requestPayload);

      window.console.log('Status response:', response);

      // Extract field values from the nested response structure using schema
      const responseData = extractFieldValues(response);
      const status = responseData.get('status') || null;

      window.console.log('Extracted response data:', responseData);
      window.console.log('Extracted status:', status);

      if (status) {
        if (status === 'RUNNING') {
          window.console.log('Job still running, continuing to poll...');
          // Continue polling - interval will handle the next call
        } else if (status === 'SUCCEEDED') {
          window.console.log('Job completed successfully!');

          // Clear polling interval
          if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current);
            pollingIntervalRef.current = null;
          }

          const downloadUrl = responseData.get('download_url');
          window.console.log('Download URL:', downloadUrl);

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

          setDownloadState('downloaded');
        } else {
          // Handle error status
          window.console.error('Job ended with status:', status);

          // Clear polling interval
          if (pollingIntervalRef.current) {
            clearInterval(pollingIntervalRef.current);
            pollingIntervalRef.current = null;
          }

          let errorMessage = `Job failed with status: ${status}`;

          if (responseData.has('error')) {
            errorMessage += `\nError: ${responseData.get('error')}`;
          }
          if (responseData.has('cause')) {
            errorMessage += `\nCause: ${responseData.get('cause')}`;
          }

          setError(errorMessage);
          // setDownloadState('error');
        }
      } else {
        throw new Error('Invalid status response format - no status found in response');
      }
    } catch (error) {
      window.console.error('Error polling job status:', error);
      let errorMessage = JSON.stringify(error, null, 2);
      setError(`Failed to poll status: ${errorMessage}`);

      // Clear polling interval
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
        pollingIntervalRef.current = null;
      }

      // setDownloadState('error');
    }
  }, [data.timeRange, options.pcapExtractorDataSource]);

  const handleDownload = async () => {
    console.log('🔥 DOWNLOAD BUTTON CLICKED - Handler executing!');
    window.console.log('=== PCAP Download Process Started ===');
    window.console.log('Button clicked - initiating download process');


    // Generate job ID
    const jobId = "run-" + Date.now();
    setDownloadState('processing');
    setError(null);

    try {
      window.console.log('Step 1: Validating configuration...');
      window.console.log('PCAP Extractor DataSource UID:', options.pcapExtractorDataSource);

      // Validate configuration
      if (!options.pcapExtractorDataSource) {
        window.console.error('Configuration validation failed: No PCAP Extractor data source configured');
        throw new Error('PCAP Extractor data source not configured');
      }
      window.console.log('✓ Configuration validation passed');

      window.console.log('Step 2: Collecting panel data...');
      const seriesData = data.series?.[0];

      // Extract only the required fields: source_file and source_packet_number
      const requiredFieldNames = ['source_file', 'source_packet_number'];
      const extractedFields = seriesData?.fields?.filter(field =>
        requiredFieldNames.includes(field.name)
      ).map(field => ({
        name: field.name,
        type: field.type,
        values: field.values ? Array.from(field.values) : []
      })) || [];

      // Validate that both required fields are present
      const foundFieldNames = extractedFields.map(field => field.name);
      const missingFields = requiredFieldNames.filter(name => !foundFieldNames.includes(name));

      if (missingFields.length > 0) {
        window.console.error('Missing required fields:', missingFields);
        throw new Error(`Missing required fields: ${missingFields.join(', ')}`);
      }

      // Transform data to group packet numbers by source file
      const sourceFileField = extractedFields.find(field => field.name === 'source_file');
      const packetNumberField = extractedFields.find(field => field.name === 'source_packet_number');

      const groupedData: { [sourceFile: string]: number[] } = {};

      if (sourceFileField && packetNumberField) {
        const sourceFiles = sourceFileField.values;
        const packetNumbers = packetNumberField.values;

        for (let i = 0; i < sourceFiles.length; i++) {
          const sourceFile = sourceFiles[i] as string;
          const packetNumber = packetNumbers[i] as number;

          if (!groupedData[sourceFile]) {
            groupedData[sourceFile] = [];
          }

          if (!groupedData[sourceFile].includes(packetNumber)) {
            groupedData[sourceFile].push(packetNumber);
          }
        }
      }

      // Prepare the query for the PCAP extractor data source
      const query = {
        refId: 'pcap-extract',
        datasource: {
          type: 'emnify-pcapextractor-datasource',
          uid: options.pcapExtractorDataSource
        },
        action: 'request',
        jobId: jobId,
        extract: groupedData,
        timeRange: data.timeRange,

      };


      window.console.log('Grouped data by source file:', groupedData);

      window.console.log('Step 3: Sending request to backend...');

      // setJobId(currentJobId);

      const backendSrv = getBackendSrv();
      const requestPayload = {
        queries: [query],
        from: data.timeRange.from.valueOf().toString(),
        to: data.timeRange.to.valueOf().toString(),
        seriesData: [] // allSeries
      };

      window.console.log('Backend request payload:', requestPayload);

      // TODO configurable?
      const apiUrl = `/api/ds/query?ds_type=emnify-pcapdownload-datasource&requestId=${Date.now()}`;
      const response = await backendSrv.post(apiUrl, requestPayload);

      window.console.log('Request response:', response);

      // Start polling for status
      window.console.log('Step 4: Starting status polling...');
      pollingIntervalRef.current = setInterval(() => {
        pollJobStatus(jobId);
      }, 10000); // Poll every 10 seconds

      // Do initial status check immediately
      pollJobStatus(jobId);

      // window.console.log('✓ Download request submitted, now polling for status');

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
      window.console.log(`Data is loading - resetting download state from "${downloadState}" to "idle"`);
      setDownloadState('idle');
      setError(null);
      // setJobId(null);

      // Clear any ongoing polling
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
        pollingIntervalRef.current = null;
      }
    }
  }, [isDataLoading, downloadState]);

  // Reset download state when data changes (e.g., on refresh, time range change, etc.)
  React.useEffect(() => {
    if (downloadState === 'downloaded' || downloadState === 'error') {
      window.console.log(`Data changed - resetting download state from "${downloadState}" to "idle"`);
      setDownloadState('idle');
      setError(null);
      // setJobId(null);

      // Clear any ongoing polling
      if (pollingIntervalRef.current) {
        clearInterval(pollingIntervalRef.current);
        pollingIntervalRef.current = null;
      }
    }
  }, [data.timeRange, data.request?.requestId, downloadState]);

  // Log button state
  React.useEffect(() => {
    window.console.log('Button state update:', {
      isConfigured,
      hasData,
      downloadState,
      isDataLoading,
      dataState: data.state,
      buttonEnabled: isConfigured && hasData && downloadState === 'idle' && !isDataLoading
    });
  }, [isConfigured, hasData, downloadState, isDataLoading, data.state]);

  // Log render
  window.console.log('Rendering Download component with current state:', {
    downloadState,
    error: !!error,
    isConfigured,
    hasData
  });

  // Get button properties based on state
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
          text: 'Downloaded',
          icon: 'check' as const,
          variant: 'success' as const,
          disabled: true
        };
      case 'error':
        return {
          text: options.text || 'Download PCAP',
          icon: 'download-alt' as const,
          variant: 'primary' as const,
          disabled: !isConfigured || !hasData || isDataLoading
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
    <div
      className={cx(
        styles.wrapper,
        css`
          width: ${width}px;
          height: ${height}px;
        `
      )}
    >
      <div className={styles.buttonContainer}>
        <Button
          variant={buttonProps.variant}
          size="md"
          icon={buttonProps.icon}
          onClick={handleDownload}
          disabled={buttonProps.disabled}
        >
          {downloadState === 'processing' && <Spinner size="sm" />}
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
