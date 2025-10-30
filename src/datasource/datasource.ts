import { DataSourceInstanceSettings, CoreApp, ScopedVars, DataQueryRequest, DataQueryResponse } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { Observable } from 'rxjs';

import { MyQuery, DataSourceOptions, DEFAULT_QUERY } from './types';

export class DataSource extends DataSourceWithBackend<MyQuery, DataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<DataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<MyQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: MyQuery, scopedVars: ScopedVars) {
    return {
      ...query,
      queryText: getTemplateSrv().replace(query.queryText, scopedVars),
    };
  }

  filterQuery(query: MyQuery): boolean {
    const action = query.action || 'request';
    
    if (action === 'request') {
      // For request action, require JobId, Bucket, and Extract data
      return !!(query.JobId && query.Bucket && query.Extract && Object.keys(query.Extract).length > 0);
    } else if (action === 'status') {
      // For status action, require executionArn
      return !!(query.executionArn);
    }
    
    return false;
  }

  query(request: DataQueryRequest<MyQuery>): Observable<DataQueryResponse> {
    // Check if any query has dashboard/panel configuration
    const modifiedRequest = {
      ...request,
      targets: request.targets.map(target => {
        // Use per-query dashboard/panel configuration if available
        if (target.dashboardUid && target.panelId !== undefined) {
          return {
            ...target,
            dashboardUid: target.dashboardUid,
            panelId: target.panelId,
          };
        }
        
        // Fallback to datasource-level configuration if no per-query config
        const instanceSettings = (this as any).instanceSettings;
        const jsonData = instanceSettings?.jsonData || {};
        const { dashboardUid, panelId } = jsonData;
        
        if (dashboardUid && panelId !== undefined) {
          return {
            ...target,
            dashboardUid,
            panelId,
          };
        }
        
        return target;
      })
    };
    
    return super.query(modifiedRequest);
  }


}
