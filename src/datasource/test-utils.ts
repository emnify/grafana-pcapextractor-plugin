// Test utilities for the dashboard panel integration
export const mockDashboardResponse = {
  dashboard: {
    id: 1,
    uid: 'test-dashboard-uid',
    title: 'Test Dashboard',
    panels: [
      {
        id: 1,
        title: 'Test Panel 1',
        type: 'graph',
        targets: [
          {
            refId: 'A',
            queryType: '',
            datasource: { type: 'prometheus', uid: 'prometheus-uid' }
          }
        ]
      },
      {
        id: 2,
        title: 'Test Panel 2',
        type: 'stat',
        targets: [
          {
            refId: 'B',
            queryType: '',
            datasource: { type: 'influxdb', uid: 'influx-uid' }
          }
        ]
      }
    ]
  }
};

export const mockSearchResponse = [
  {
    id: 1,
    uid: 'dashboard-1',
    title: 'Dashboard 1',
    type: 'dash-db'
  },
  {
    id: 2,
    uid: 'dashboard-2', 
    title: 'Dashboard 2',
    type: 'dash-db'
  }
];

export const createMockDataSource = (options: any = {}) => {
  return {
    instanceSettings: {
      jsonData: {
        path: '/api/v1',
        dashboardUid: 'test-dashboard-uid',
        panelId: 1,
        ...options
      }
    }
  };
};