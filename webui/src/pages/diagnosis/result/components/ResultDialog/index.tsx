import { Dialog, Divider, List } from "@alifd/next";
import styles from "./index.module.css";
import { Grid } from "@alifd/next";

export interface ResultDialogData {
  type: string
  data: DiagnosisCluster | DiagnosisNode | DiagnosisLink
}


interface ResultDialogProps {
  visible: boolean;
  onClose: () => void
  data?: ResultDialogData
}

const levelToString = (level: SuspicionLevel) => {
  switch (level) {
    case 0:
      return "INFO";
    case 1:
      return "WARNING";
    case 2:
      return "CRITICAL"
    case 3:
      return "FATAL";
    default:
      return "UNKNOWN";
  }
}

const suspicionColors = {
  0: '#30BD61',
  1: '#FFB369',
  2: '#F76D76',
  3: '#F76D76'
}

const makeAttributes = (data?: ResultDialogData): string[][] => {
  if (!data) return [];
  switch(data.type) {
    case "node":
      const n = data.data as DiagnosisNode;
      return [
        ['Node ID', n.id],
        ['Type', n.type]
      ]
    case "edge":
      const e = data.data as DiagnosisLink;
      return [
        ['Edge ID', e.id],
        ['Type', e.type],
        ["Action", e.action],
        ["Output Interface (Sender Node)", e.source_attributes.if],
        ["Input Interface (Receiver Node)", e.destination_attributes.if],
        ["Packet Source", e.packet.source],
        ["Packet Destination", e.packet.destination],
        ["Packet Destination Port", e.packet.dport],
        ["Packet Protocol", e.packet.protocol]
      ]
    default:
      return [];
  }
};

const makeAttributeItem = (i: string[], idx: number) => {
  return <List.Item
    id={idx.toString()}
    style={{ padding: '2px' }}
    className={styles.attributeItem}
    title={<span className={styles.attributeTitle}>{i[0]}</span>}>
    <span className={styles.attributeItem}>{i[1]}</span>
  </List.Item>
};

const makeSuspicionItem = (i: Suspicion, idx: number) => {
  return <List.Item
    key={idx.toString()}
    style={{ padding: '2px' }}
    className={styles.attributeItem}>
    <Grid.Row>
      <Grid.Col span={3}
      style={{backgroundColor: suspicionColors[i.level] ? suspicionColors[i.level] : '#525252' }}
      className={styles.suspicionIcon}>
        {levelToString(i.level)}
      </Grid.Col>
      <Grid.Col span={24} className={styles.attributeItem}>{i.message}</Grid.Col>
    </Grid.Row>
  </List.Item>
};

const ResultDialog: React.FC<ResultDialogProps> = (props) => {
  const { visible, data, onClose } = props;
  const attributes = makeAttributes(data)
  return (
    <Dialog
      v2
      title="详情"
      footerActions={['ok']}
      visible={visible}
      onClose={onClose}
      onOk={onClose}
    >
      <List
        divider={false}
        dataSource={attributes}
        renderItem={makeAttributeItem} />
      <Divider />
      <List
        divider={false}
        dataSource={data?.data.suspicions || []}
        emptyContent={<span>没有异常</span>}
        renderItem={makeSuspicionItem}
      />
    </Dialog>
  );
};
export default ResultDialog;
