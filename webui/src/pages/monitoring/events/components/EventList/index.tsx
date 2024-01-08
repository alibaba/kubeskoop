import { EventData } from "@/services/event";
import { List, Tag } from "@alifd/next";
import styles from './index.module.css'

interface EventListProps {
  data: EventData[]
};

const renderListItem = (ev: EventData): JSX.Element => {
  // convert nanosecond timestamp to date
  const date = new Date(ev.timestamp / 1000000);
  return (
    <div className={styles.eventItem}>
      <div className={styles.bar} />
      <div className={styles.eventInfo}>
        <div>{date.toISOString()}</div>
        <div className={styles.eventType}>{ev.type}</div>
      </div>
      <div>
        <div>
          {ev.labels.map(i => <Tag className={styles.tag} size="small" color="blue">{`${i.name}: ${i.value}`}</Tag>)}
        </div>
        <div className={styles.eventMessage}>{ev.msg}</div>
      </div>
    </div>
  )
};

const EventList: React.FC<EventListProps> = (props: EventListProps): JSX.Element => {
  return (<div>
    <List dataSource={props.data} renderItem={renderListItem} />
  </div>);
};

export default EventList;
