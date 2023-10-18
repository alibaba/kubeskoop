import * as React from 'react';
import { Card } from '@alifd/next';
import { Chart, Geom } from 'bizcharts';
import mock from './mock.js';
import styles from './index.module.css';


interface CardConfig {
  title: string | React.ReactNode;
  subTitle: string | React.ReactNode;
  value: string;
  chartData: number[];
  des: string;
  rate: string;
  chartHeight: number;
}

const DEFAULT_DATA: CardConfig = {
  title: '',
  subTitle: '访问量',
  value: mock.value,
  chartData: mock.saleList,
  des: '周同比:',
  rate: '12.0',
  chartHeight: 100,
};
interface FusionCardAreaChartProps {
  cardConfig?: CardConfig;
}
const FusionCardAreaChart: React.FunctionComponent<FusionCardAreaChartProps> = (props = DEFAULT_DATA): JSX.Element => {
  const {
    cardConfig = DEFAULT_DATA,
  } = props;
  const { title, subTitle, value, chartData, des, rate, chartHeight } = cardConfig;

  return (
    <Card free className={styles.areaChart}>
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
          <Geom type="line" position="date*value" color="#00D6CB" shape="smooth" opacity={1} />
          <Geom type="area" position="date*value" color="#00D6CB" shape="smooth" opacity={0.1} />

        </Chart>
      </Card.Content>
    </Card>
  );
};

export default FusionCardAreaChart;
