# PCAP Extractor

This custom emnify plugin facilitates with the extraction and download of PCAP files based on PCAP Parser / IMSI-based Observability.

It is built as a Grafana app plugin, to package the two required plugins:
- _PCAP Extractor_ data source 
- _PCAP Download_ button panel

This plugin also requires the PCAP Extractor Step Function to be deployed.

## Configuration

The data source shall be configured for automated deployment using Grafana provisioning:

```yaml
datasources:
  - name: 'PCAP Extractor'
    type: 'emnify-pcapextractor-datasource'
    jsonData:
      s3Bucket: my-pcap-extractor
      stepFunctionArn: arn:aws:states:us-onfire-1:12345678912:stateMachine:my-pcap-extractor
```

## Usage

- Query PCAP data, make sure that results include columns `source_file` and `source_packet_number`.
- Add a panel of type `PCAP download`.
- Configure the panel and pick the `emnify-pcap-extractor` data source that should be available after successful provisioning.

| Panel appearance                                        | Panel configuration                                            |
|---------------------------------------------------------|----------------------------------------------------------------|
| ![Download button](https://github.com/emnify/grafana-pcapextractor-plugin/blob/main/src/datasource/img/panel-button.png?raw=true) | ![Download button](https://github.com/emnify/grafana-pcapextractor-plugin/blob/main/src/datasource/img/panel-configuration.png?raw=true) |


## Development

### Frontend

1. Install dependencies

   ```bash
   npm install
   ```

2. Build plugin in development mode and run in watch mode

   ```bash
   npm run dev
   ```

3. Build plugin in production mode

   ```bash
   npm run build
   ```

4. Run the tests (using Jest)

   ```bash
   # Runs the tests and watches for changes, requires git init first
   npm run test

   # Exits after running all the tests
   npm run test:ci
   ```

5. Spin up a Grafana instance and run the plugin inside it (using Docker)

   ```bash
   npm run server
   ```

6. Run the E2E tests (using Playwright)

   ```bash
   # Spins up a Grafana instance first that we tests against
   npm run server

   # If you wish to start a certain Grafana version. If not specified will use latest by default
   GRAFANA_VERSION=11.3.0 npm run server

   # Starts the tests
   npm run e2e
   ```

7. Run the linter

   ```bash
   npm run lint

   # or

   npm run lint:fix
   ```
