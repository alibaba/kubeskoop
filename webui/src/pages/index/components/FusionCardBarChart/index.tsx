import * as React from 'react';
import { Card } from '@alifd/next';
import { Chart, Geom } from 'bizcharts';
import mock from './mock.js';

import styles from './index.module.css';

interface ChartItem {
  date?: string;
  value?: number;
}

interface CardConfig {
  title?: string | React.ReactDOM;
  subTitle?: string | React.ReactDOM;
  value?: string;
  chartData?: ChartItem[];
  des?: string;
  rate?: number;
  chartHeight?: number;
}

const DEFAULT_DATA: CardConfig = {
  subTitle: '总销售额',
  value: mock.value,
  chartData: mock.saleList,
  des: '周同比:',
  rate: 10.1,
  chartHeight: 100,
};

export interface FusionCardBarChartProps {
  cardConfig?: CardConfig;
}

const FusionCardBarChart: React.FunctionComponent<FusionCardBarChartProps> = (props: FusionCardBarChartProps): JSX.Element => {
  const {
    cardConfig = DEFAULT_DATA,
  } = props;

  const { title, subTitle, value, chartData, des, rate, chartHeight } = cardConfig;

  return (
    <Card free>
      {
        title ? (
          <>
            <Card.Header title={title} />
            <Card.Divider />
          </>
        ) : null
      }
      <Card.Content>
        <div className={styles.cardSubTitle}>{subTitle}</div>
        <div className={styles.cardValue}>{value}</div>
        <div className={styles.cardDes}>{des}<span>{rate}↑</span></div>
        <Chart
          width={10}
          height={chartHeight}
          data={chartData}
          scale={{
            date: {
              range: [0, 1],
            },
          }}
          forceFit
          padding={['auto', '16']}
        >
          <Geom type="interval" position="date*value" color="#29A5FF" />
        </Chart>
      </Card.Content>
    </Card>
  );
};

export default FusionCardBarChart;
