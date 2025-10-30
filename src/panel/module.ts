import { PanelPlugin } from '@grafana/data';
import { getDataSourceSrv } from '@grafana/runtime';
import { PcapExtractorOptions } from './types';
import { Download } from './components/Download';

export const plugin = new PanelPlugin<PcapExtractorOptions>(Download).setPanelOptions((builder) => {
  return builder

    .addSelect({
      path: 'pcapExtractorDataSource',
      name: 'PCAP Extractor Data Source',
      description: 'Select the PCAP extractor data source',
      settings: {
        options: [],
        getOptions: async () => {
          try {
            const dataSourceSrv = getDataSourceSrv();
            const dataSources = dataSourceSrv.getList();

            const options = dataSources
              .filter(ds => ds.type === 'emnify-pcapextractor-datasource')
              .map(ds => ({
                label: ds.name,
                value: ds.uid,
                description: ds.type,
              }));

            return options;
          } catch (error) {
            console.error('Error fetching PCAP extractor data sources:', error);
            return [];
          }
        },
      },
    })
    .addTextInput({
      path: 'text',
      name: 'Button Text',
      description: 'Text to display on the download button',
      defaultValue: 'Download PCAP',
    })
});
