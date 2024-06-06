import PageHeader from '@/components/PageHeader';
import WebFrameCard from '@/components/WebFrameCard';
import { useEffect, useMemo, useState } from 'react';
import Exception from '@/components/Exception';
import { Card, Loading, Message, Tab } from '@alifd/next';
import store from '@/store'
import { definePageConfig, useParams } from "ice";

export default function Dashboard() {
  const params = useParams();
  const type = params.type;
  const [loading, setLoading] = useState(true)

  const [dashboardConfig, dashboardConfigDispatcher] = store.useModel('dashboard');
  const effectsState = store.useModelEffectsState('dashboard');
  const url = useMemo(() => {
    switch(type) {
      case 'pod':
        return dashboardConfig.pod_dashboard_url;
      case 'node':
        return dashboardConfig.node_dashboard_url;
      default:
        return '';
    }
  }, [type, dashboardConfig])

  useEffect(() => {
    dashboardConfigDispatcher.fetchDashboardConfig()
      .then(() => {
        localStorage.setItem("SearchBar_Hidden", "true")
        localStorage.setItem("grafana.navigation.docked", "false")
        if (effectsState.fetchDashboardConfig.error) {
          Message.error(`Error when fetching dashboard config: ${effectsState.fetchDashboardConfig.error.response.data.error}`)
        }
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  return (
    <div>
      <Card contentHeight={"auto"} free>
            {url? (
              <WebFrameCard src={url} />
            ) : (
              <Loading visible={loading} style={{ display: 'block' }}>
                <Exception title="Dashboard not configured." description="Please configure dashboard url to show this page." />
              </Loading>
            )}
      </Card>
    </div>
  );
}

export const pageConfig = definePageConfig(() => {
  return {
    title: 'Dashboard',
  };
});
