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
        <Exception
          title="404"
          image="https://img.alicdn.com/tfs/TB14c1VoET1gK0jSZFhXXaAtVXa-200-200.png"
          description="Oops! Page not found."
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
