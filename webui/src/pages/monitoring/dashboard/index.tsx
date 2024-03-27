import PageHeader from '@/components/PageHeader';
import WebFrameCard from '@/components/WebFrameCard';
import { useEffect, useState } from 'react';
import Exception from '@/components/Exception';
import { Loading, Message } from '@alifd/next';
import store from '@/store'
import { definePageConfig } from "ice";

export default function Dashboard() {
  const [loading, setLoading] = useState(true)

  const [dashboardConfig, dashboardConfigDispatcher] = store.useModel('dashboard');
  const effectsState = store.useModelEffectsState('dashboard');

  useEffect(() => {
      dashboardConfigDispatcher.fetchDashboardConfig()
      .then(() => {
        localStorage.setItem("SearchBar_Hidden", "true")
        localStorage.setItem("grafana.navigation.docked", "false")
        if(effectsState.fetchDashboardConfig.error) {
          Message.error(`Error when fetching dashboard config: ${effectsState.fetchDashboardConfig.error.response.data.error}`)
        }
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  const onSubmit = async ({ url }) => {
    return await dashboardConfigDispatcher.updateDashboardConfig({ 'metrics_url': url })
    .then(() => {
      if(effectsState.updateDashboardConfig.error) {
        Message.error(`Error when submitting dashboard config: ${effectsState.updateDashboardConfig.error.response.data.error}`)
      }
    })
  }

  return (
    <div>
      <PageHeader
        title='Monitoring'
        breadcrumbs={[{ name: 'Console' }, { name: 'Monitoring' }, {name: 'Dashboard'}]}
      />
      {dashboardConfig.metrics_url ? (
        <div className='web-frame'>
          <WebFrameCard src={dashboardConfig.metrics_url} />
        </div>
      ) : (
        <Loading visible={loading} style={{ display: 'block' }}>
        <Exception title="Dashboard not configured." description="Please configure dashboard url to show this page." />
        </Loading>
      )}
    </div>
  );
}

export const pageConfig = definePageConfig(() => {
  return {
    title: 'Dashboard',
  };
});
