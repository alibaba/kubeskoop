import { definePageConfig } from 'ice';
import PageHeader from '@/components/PageHeader';
import WebFrameCard from '../../components/WebFrameCard';
import URLDialog from '@/components/URLDialog';
import { useState } from 'react';

export default function Dashboard() {
  const [visible, setVisible] = useState(false);

  const onSubmit = async ({url}) => {
    await new Promise((resolve) => {
      setTimeout(resolve, 1000);
    }).then(() => {
      console.log('ok')
    })
  }

  return (
    <div>
        <PageHeader
          title='监控'
          breadcrumbs={[{ name: 'Console' }, { name: '监控' }]}
        />
        <WebFrameCard src='/diagnosis' onSetting={() => setVisible(true)}/>
        <URLDialog visible={visible} onVisibleChange={setVisible} onSubmit={onSubmit} url="https://www.baidu.com"/>
    </div>
  );
}

export const pageConfig = definePageConfig(() => {
  return {
    title: 'KubeSkoop Web Console',
  };
});
