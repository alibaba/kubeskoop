import PageHeader from '@/components/PageHeader';
import WebFrameCard from '@/components/WebFrameCard';
import { useEffect, useState } from 'react';
import Exception from '@/components/Exception';
import { Button, Loading, Message } from '@alifd/next';
import store from '@/store'

export default function Events() {
  const [visible, setVisible] = useState(false);
  const [loading, setLoading] = useState(true)

  const [dashboardConfig, dashboardConfigDispatcher] = store.useModel('dashboard');
  const effectsState = store.useModelEffectsState('dashboard');

  useEffect(() => {
    dashboardConfigDispatcher.fetchDashboardConfig()
      .then(() => {
        if (effectsState.fetchDashboardConfig.error) {
          Message.error(`Error when fetching dashboard config: ${effectsState.fetchDashboardConfig.error.response.data.error}`)
        }
      })
      .finally(() => {
        setLoading(false)
      })
  }, [])

  const onSubmit = async ({ url }) => {
    return await dashboardConfigDispatcher.updateDashboardConfig({ 'event_url': url })
      .then(() => {
        if (effectsState.updateDashboardConfig.error) {
          Message.error(`Error when submitting dashboard config: ${effectsState.updateDashboardConfig.error.response.data.error}`)
        }
      })
  }

  return (
    <div>
      <PageHeader
        title='事件'
        breadcrumbs={[{ name: 'Console' }, { name: '监控' }, { name: '事件' }]}
      />
      {dashboardConfig.event_url ? (
        <div className='web-frame'>
          <WebFrameCard src={dashboardConfig.event_url} onSetting={() => setVisible(true)} />
        </div>
      ) : (
        <Loading visible={loading} style={{ display: 'block' }}>
          <Exception title="未配置大盘链接" description="请配置大盘链接后使用" extra={<Button type="primary" onClick={() => setVisible(true)}>配置</Button>} />
        </Loading>
      )}
    </div>
  );
}
