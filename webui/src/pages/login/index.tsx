import { ResponsiveGrid } from '@alifd/next';
import { definePageConfig } from 'ice';
import LoginBlock from './components/LoginBlock';
import styles from './index.module.css';

const { Cell } = ResponsiveGrid;

const Login: React.FC = () => {
  return (
    <div className={styles.container}>
      <div className={styles.content}>
        <ResponsiveGrid gap={20}>
          <Cell colSpan={12}>
            <LoginBlock />
          </Cell>
        </ResponsiveGrid>
      </div>
    </div>
  );
};

export const pageConfig = definePageConfig(() => {
  return {
    title: '登录',
  };
});

export default Login;
