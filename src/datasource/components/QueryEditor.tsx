import React from 'react';
import { Alert } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { DataSourceOptions, Query } from '../types';

type Props = QueryEditorProps<DataSource, Query, DataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  return (
    <div>
      <Alert title="PCAP Extractor Data Source" severity="warning">
        This data source is not meant to be invoked directly, but from the PCAP Download button.
      </Alert>
    </div>
  );
}
