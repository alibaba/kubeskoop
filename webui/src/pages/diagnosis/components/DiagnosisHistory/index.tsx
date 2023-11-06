import { Table } from "@alifd/next";

interface DiagnosisRow {
  id: string,
  time: string;
  src: string,
  dst: string,
  port: number,
  protocol: string
  status: string | JSX.Element
  action: JSX.Element | null
}

interface DiagnosisHistoryProps {
  data: DiagnosisRow[];
}

const columns = [
  {
    title: "ID",
    dataIndex: "id",
    width: 80
  },
  {
    title: "时间",
    dataIndex: "time",
  },
  {
    title: "源IP",
    dataIndex: "src",
  },
  {
    title: "目的IP",
    dataIndex: "dst",
  },
  {
    title: "端口",
    dataIndex: "port",
  },
  {
    title: "协议",
    dataIndex: "protocol",
  },
  {
    title: "状态",
    dataIndex: "status",
  },
  {
    title: "操作",
    dataIndex: "action"
  }
];

const DiagnosisHistory: React.FunctionComponent<DiagnosisHistoryProps> = (props: DiagnosisHistoryProps): JSX.Element => {
  const { data } = props;

  return (
    <div className="diagnosis-history">
      <Table.StickyLock columns={columns} dataSource={data} />
    </div>
  );
};
export default DiagnosisHistory;
