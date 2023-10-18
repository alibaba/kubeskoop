import * as React from 'react';
import { Radio, Card, Box } from '@alifd/next';
import { Chart, Geom, Coord, Axis, Legend, Guide } from 'bizcharts';

import styles from './index.module.css';

const { useState } = React;
const { Html } = Guide;

interface ChartItem {
  type?: string;
  value?: number;
  title?: string;
}

interface CardConfig {
  title?: string;
  value?: number;
  chartData?: ChartItem[];
  chartHeight?: number;
}

const DEFAULT_DATA: CardConfig = {
  title: '销售额类别占比',
  value: 183112,
  chartData: [
    {
      type: '类别一事例一',
      value: 40,
      title: '类别一事例一 | 40.00%     ¥4,544',
    },
    {
      type: '类别一事例二',
      value: 21,
      title: '类别一事例二 | 22.12%     ¥2,344',
    },
    {
      type: '类别一事例三',
      value: 17,
      title: '类别一事例三 | 16.59%     ¥3,512',
    },
    {
      type: '类别一事例四',
      value: 13,
      title: '类别一事例四 | 13.11%     ¥2,341',
    },
    {
      type: '类别一事例五',
      value: 9,
      title: '类别一事例五 |  9.29%     ¥1,231',
    },
  ],
  chartHeight: 500,
};

export interface FusionCardLineChartProps {
  cardConfig?: CardConfig;
}

const FusionCardLineChart: React.FunctionComponent<FusionCardLineChartProps> = (props: FusionCardLineChartProps): JSX.Element => {
  const {
    cardConfig = DEFAULT_DATA,
  } = props;

  const { title, value, chartData, chartHeight } = cardConfig;

  const [type, setType] = useState('one');
  const changeType = (key: string) => setType(key);


  return (
    <Card free>
      <Card.Header title={title} />
      <Card.Divider />
      <Card.Content>
        <Box align="center">
          <Radio.Group
            shape="button"
            value={type}
            onChange={changeType}
            className={styles.radioGroup}
          >
            <Radio value="one" className={styles.radioFlex}>
              类目一
            </Radio>
            <Radio value="two" className={styles.radioFlex}>
              类目二
            </Radio>
            <Radio value="three" className={styles.radioFlex}>
              类目三
            </Radio>
          </Radio.Group>
        </Box>
        <Chart width={10} height={chartHeight} forceFit data={chartData} padding={['auto', 'auto']}>
          <Coord type="theta" radius={0.75} innerRadius={0.6} />
          <Axis name="percent" />
          <Legend
            position="bottom"
            layout="vertical"
            offsetY={-30}
            textStyle={{
              fill: '#666',
              fontSize: 14,
            }}
            itemMarginBottom={24}
          />
          <Guide>
            <Html
              position={['50%', '50%']}
              // eslint-disable-next-line max-len
              html={`<div style='color:#333;font-size:16px;text-align: center;width: 113px;'>销售额<br><span style='color:#333;font-family: Roboto-Bold;font-size:24px'>¥ ${value}</span></div>`}
              alignX="middle"
              alignY="middle"
            />
          </Guide>
          <Geom
            type="intervalStack"
            position="value"
            color="title"
            style={{
              lineWidth: 1,
              stroke: '#fff',
            }}
          />
        </Chart>
      </Card.Content>
    </Card>
  );
};

export default FusionCardLineChart;
