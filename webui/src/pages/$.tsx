import * as React from 'react';
import { ResponsiveGrid } from '@alifd/next';
import PageHeader from '@/components/PageHeader';
import Exception from '@/components/Exception';
import { definePageConfig } from 'ice';

const { Cell } = ResponsiveGrid;

const FeedbackNotFound = () => {
  return (
    <ResponsiveGrid gap={20}>
      <Cell colSpan={12}>
        <PageHeader
          title="404页面"
          breadcrumbs={[
            { name: 'Feedback页面' },
            { name: '结果页面' },
            { name: '404页面' },
          ]}
          description="404页面描述"
        />
      </Cell>

      <Cell colSpan={12}>
        <Exception
          statusCode="404"
          image="https://img.alicdn.com/tfs/TB14c1VoET1gK0jSZFhXXaAtVXa-200-200.png"
          description="服务器好像挂了你要等会了"
        />
      </Cell>
    </ResponsiveGrid>
  );
};

export const pageConfig = definePageConfig(() => {
  return {
    title: '404',
  };
});

export default FeedbackNotFound;
