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
    title: "Time",
    dataIndex: "time",
  },
  {
    title: "Source Address",
    dataIndex: "src",
  },
  {
    title: "Destination Address",
    dataIndex: "dst",
  },
  {
    title: "Port",
    dataIndex: "port",
  },
  {
    title: "Protocol",
    dataIndex: "protocol",
  },
  {
    title: "Status",
    dataIndex: "status",
  },
  {
    title: "Actions",
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
