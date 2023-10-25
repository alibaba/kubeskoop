import * as React from 'react';
import { Card } from '@alifd/next';
import styles from './index.module.css';


export interface ExceptionProps {
  title: string;
  image?: string;
  description?: string;
  extra: JSX.Element;
}

const Exception: React.FC<ExceptionProps> = (props: ExceptionProps) => {
  const {
    title,
    image,
    description,
    extra
  } = props;

  return (
    <Card free className={styles.exception}>
      <div>
        { image ? <img src={image} className={styles.exceptionImage}/> : null }
        <h1 className={styles.statusCode}>{title}</h1>
        { description ? <div className={styles.description}>{description}</div> : null}
        <div>{extra}</div>
      </div>
    </Card>
  );
};

export default Exception;
