import styles from './index.module.css'
import { Card, Button, Icon } from '@alifd/next'

export interface WebFrameProps {
    src: string
    onSetting?: () => void
}

const WebFrameCard: React.FC<WebFrameProps> = (props: WebFrameProps): JSX.Element => {
    return (
        <Card contentHeight="auto" >
            <Card.Content className={styles.cardContent}>
                <iframe className={styles.webFrame} src={props.src}></iframe>
            </Card.Content>
            <Card.Actions className={styles.cardAction}>
                <Button text onClick={props.onSetting}>
                    <Icon type="set" />设置
                </Button>
            </Card.Actions>
        </Card>
    )
}

export default WebFrameCard;
