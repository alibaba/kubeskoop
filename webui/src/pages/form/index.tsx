import BasicForm from './components/BasicForm';
import PageHeader from '@/components/PageHeader';
import { submitForm } from '@/services/form';
import { Message, ResponsiveGrid } from '@alifd/next';
import { definePageConfig } from 'ice';

const { Cell } = ResponsiveGrid;

export default function FormBasic() {
  const onSubmit = async (values: Record<string, any>) => {
    submitForm(values).then(() => {
      Message.success('提交成功');
    });
  };

  return (
    <ResponsiveGrid gap={20}>
      <Cell colSpan={12}>
        <PageHeader
          title="单列基础表单"
          description="单列基础表单描述信息"
          breadcrumbs={[{ name: '表单页面' }, { name: '单列基础表单' }]}
        />
      </Cell>

      <Cell colSpan={12}>
        <BasicForm onSubmit={onSubmit} />
      </Cell>
    </ResponsiveGrid>
  );
}

export const pageConfig = definePageConfig(() => {
  return {
    auth: ['admin', 'user'],
    title: '表单页',
  };
});
