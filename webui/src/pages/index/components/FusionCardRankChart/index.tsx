import * as React from 'react';
import { Card, ResponsiveGrid, Box, Divider } from '@alifd/next';

import styles from './index.module.css';

const { Cell } = ResponsiveGrid;

interface DataItem {
  name?: string;
  rate?: string;
  color?: string;
}

interface CardConfig {
  title?: string;
  dataSource?: DataItem[];
}

export interface FusionCardRankChartProps {
  cardConfig?: CardConfig;
}

const DEFAULT_DATA: CardConfig = {
  title: '区域销售',
  dataSource: [
    { name: '亚洲', rate: '40%', color: '#2B7FFB' },
    { name: '欧洲', rate: '30%', color: '#00D6CB' },
    { name: '南非', rate: '20%', color: '#F0C330' },
    { name: '美洲', rate: '10%', color: '#3840D9' },
  ],
};

const FusionCardRankChart: React.FunctionComponent<FusionCardRankChartProps> = (props: FusionCardRankChartProps): JSX.Element => {
  const { cardConfig = DEFAULT_DATA } = props;
  const { title, dataSource } = cardConfig;
  return (
    <Card free>
      <Card.Header title={title} />
      <Card.Divider />
      <Card.Content style={{ margin: 0, padding: 0 }}>
        <ResponsiveGrid>
          <Cell colSpan={6}>
            <div className={styles.hisMap} />
          </Cell>
          <Cell colSpan={3}>
            <Box justify="flex-start" spacing={20} className={styles.histogram}>
              {dataSource &&
                dataSource.map((item, idx) => (
                  <Box key={idx} justify="flex-start" spacing={5}>
                    <div className={styles.hisTitle}>{item.name}</div>
                    <Box direction="row">
                      <div style={{ backgroundColor: item.color, width: item.rate }} />
                      <div className={styles.hisRate}>{item.rate}</div>
                    </Box>
                  </Box>
                ))}
            </Box>
          </Cell>
          <Cell colSpan={3}>
            <Box direction="row" className={styles.subCard}>
              <Divider direction="ver" className={styles.subDiv} />
              <div className={styles.subBody}>
                <div className={styles.subName}>亚洲</div>
                <Divider direction="hoz" />
                <Box
                  className={styles.subMain}
                  spacing={20}
                  direction="row"
                  align="center"
                  justify="center"
                >
                  <Box>
                    <div className={styles.subTypeName}>商品类目1</div>
                    <div className={styles.subTypeValue}>6,123</div>
                  </Box>
                  <Divider direction="ver" className={styles.subMainDiv} />
                  <Box>
                    <div className={styles.subTypeName}>商品类目2</div>
                    <div className={styles.subTypeValue}>132,4</div>
                  </Box>
                </Box>
                <Box
                  className={styles.subFooter}
                  direction="column"
                  justify="center"
                  align="center"
                >
                  <div>周同比</div>
                  <div>45%↑</div>
                </Box>
              </div>
            </Box>
          </Cell>
        </ResponsiveGrid>
      </Card.Content>
    </Card>
  );
};

export default FusionCardRankChart;
