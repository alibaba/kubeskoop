import * as React from 'react';
import { Breadcrumb, Box, Typography, Icon } from '@alifd/next';
import styles from './index.module.css';

export interface PageHeaderProps {
  breadcrumbs?: Array<{ name: string; path?: string }>;
  title?: string;
  description?: string;
}

const PageHeader: React.FC<PageHeaderProps> = (props: PageHeaderProps) => {
  const { breadcrumbs, title, description, ...others } = props;
  return (
    <Box spacing={8} className={styles.pageHeader} {...others}>
      {
        breadcrumbs && breadcrumbs.length > 0 ? (
          <Breadcrumb className={styles.breadcrumbs} separator=" / ">
            {
              breadcrumbs.map((item, idx) => (
                <Breadcrumb.Item key={idx} link={item.path}>{item.name}</Breadcrumb.Item>
              ))
            }
          </Breadcrumb>
        ) : null
      }

      {
        title && (
          <Typography.Text className={styles.title}>{title} </Typography.Text>
        )
      }


      {
        description && (
          <Typography.Text className={styles.description}>{description}</Typography.Text>
        )
      }
    </Box>
  );
};

export default PageHeader;
