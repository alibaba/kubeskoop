import * as React from 'react';
import { Button, Message, Card } from '@alifd/next';
import styles from './index.module.css';
import { useInterval } from '@/hooks/useInterval';

const { useState } = React;

interface DetailProcessFunc {
  (): any;
}

export interface SuccessDetailProps {
  statusCode?: string;
  description?: string;
  image?: string;
  buttonBackDesc?: string;
  buttonContinueDesc?: string;
  countDownSeconds?: number;
  onButtonBack?: DetailProcessFunc;
  onButtonContinue?: DetailProcessFunc;
}

export default function SuccessDetail(props: SuccessDetailProps) {
  const {
    statusCode = '提交成功',
    description = 's 后自动跳转至工单页',
    image = 'https://img.alicdn.com/tfs/TB1UOSVoqL7gK0jSZFBXXXZZpXa-73-72.png',
    buttonBackDesc = '返回列表',
    buttonContinueDesc = '继续创建',
    countDownSeconds = 5,
    onButtonBack = null,
    onButtonContinue = null,
  } = props;

  const [second, setSecond] = useState(countDownSeconds);

  const goBackHandle = () => {
    if (onButtonBack) {
      onButtonBack();
    } else {
      Message.notice('返回列表函数响应');
    }
  };

  const goContinueHandle = () => {
    if (onButtonContinue) {
      onButtonContinue();
    } else {
      Message.notice('继续创建函数响应');
    }
  };

  useInterval(() => {
    setSecond(second - 1);
    if (second <= 0) {
      goBackHandle();
    }
  }, second >= 0 ? 1000 : null);

  return (
    <Card free className={styles.successDetail}>
      <div>
        <img src={image} className={styles.exceptionImage} alt="img" />
        <h1 className={styles.statusCode}>{statusCode}</h1>
        <div className={styles.description}>{`${second > 0 ? second : 0}${description}`}</div>
        <div className={styles.operationWrap}>
          <Button type="primary" onClick={goBackHandle} className={styles.mainAction}>{buttonBackDesc}</Button>
          <Button onClick={goContinueHandle}>{buttonContinueDesc}</Button>
        </div>
      </div>
    </Card>
  );
}
