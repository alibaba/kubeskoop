import { EventData } from "@/services/event";
import { Button, List, Tag } from "@alifd/next";
import styles from './index.module.css'

interface EventListProps {
  data: EventData[]
};

const EventMessage = ({msg}): JSX.Element => {
  return <div>
      <span className={styles.eventMessage}>{msg}</span>
    </div>;
};

const renderListItem = (ev: EventData, i: number): JSX.Element => {
  // convert nanosecond timestamp to date
  const date = new Date(ev.timestamp / 1000000);
  return (
    <div className={styles.eventItem} key={i.toString()}>
      <div className={styles.bar} />
      <div className={styles.eventInfo}>
        <div>{date.toISOString()}</div>
        <div className={styles.eventType}>{ev.type}</div>
      </div>
      <div>
        <div>
          {ev.labels.filter(i => i.value !== '').map(i => <Tag key={i.name} className={styles.tag} size="small" color="blue">{`${i.name}: ${i.value}`}</Tag>)}
        </div>
        <EventMessage msg={ev.msg}/>
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
