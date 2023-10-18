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
  rate?: string;
  chartHeight?: number;
}

const DEFAULT_DATA: CardConfig = {
  subTitle: '门店活动效果',
  value: mock.value,
  chartData: mock.saleList,
  des: '周同比:',
  rate: '10.1',
  chartHeight: 100,
};

export interface FusionCardLineChartProps {
  cardConfig?: CardConfig;
}

const FusionCardLineChart: React.FunctionComponent<FusionCardLineChartProps> = (props: FusionCardLineChartProps): JSX.Element => {
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
          padding={['auto', '0']}
        >
          <Geom type="line" position="date*value" shape="smooth" color="#2B7FFB" />
          <Geom type="area" position="date*value" shape="smooth" color="#2B7FFB" opacity={0.1} />
          <Geom type="line" position="date*num" shape="smooth" color="#00D6CB" opacity={1} />
          <Geom type="area" position="date*num" shape="smooth" color="#00D6CB" opacity={0.1} />
        </Chart>
      </Card.Content>
    </Card>
  );
};

export default FusionCardLineChart;
