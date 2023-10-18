import * as React from 'react';
import { Link } from 'ice';
import styles from './index.module.css';

export interface ILogoProps {
  image?: string;
  text?: string;
  url?: string;
  imageStyle?: React.CSSProperties;
  textStyle?: React.CSSProperties;
}

export default function Logo({ image, text, url, imageStyle, textStyle }: ILogoProps) {
  return (
    <div>
      <Link to={url || '/'} className={styles.logo}>
        {image && <img src={image} alt="logo" style={imageStyle} />}
        <span style={textStyle}>{text}</span>
      </Link>
    </div>
  );
}
