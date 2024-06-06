import styles from './index.module.css'

export interface WebFrameProps {
    src: string
    onSetting?: () => void
}

const WebFrameCard: React.FC<WebFrameProps> = (props: WebFrameProps): JSX.Element => {
    return (
        <iframe className={styles.webFrame} src={props.src}></iframe>
    )
}

export default WebFrameCard;
