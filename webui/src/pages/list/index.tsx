import { definePageConfig } from 'ice';
import { ResponsiveGrid } from '@alifd/next';
import React from 'react';
import PageHeader from '@/components/PageHeader';
import MultiColFilterTable from './components/MultiColFilterTable';

const { Cell } = ResponsiveGrid;

const TableList: React.FC = () => {
  return (
    <ResponsiveGrid gap={20}>
      <Cell colSpan={12}>
        <PageHeader
          title="多列过滤"
          breadcrumbs={[
            { name: '列表页面' },
            { name: '多列过滤' },
          ]}
          description="多列过滤页面描述信息"
        />
      </Cell>

      <Cell colSpan={12}>
        <MultiColFilterTable />
      </Cell>
    </ResponsiveGrid>
  );
};

export default TableList;

export const pageConfig = definePageConfig(() => {
  return {
    auth: ['admin'],
    title: '列表页',
  };
});
