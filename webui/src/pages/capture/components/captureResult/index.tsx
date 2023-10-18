import {Button, Table, Icon} from '@alifd/next';
import {useState} from "react";
import moment from 'moment';

interface CaptureListProps {

}



const CaptureForm: React.FunctionComponent<CaptureListProps> = (props: CaptureListProps) => {

  const render = (value, index, record) => {
    if (record.task_status === "success") {
      return <a href="javascript:;">Download</a>;
    } else if (record.task_status === "running") {
      return <span style={{color: "green"}}>Running<a href="javascript:;">  Logs</a></span>;
    } else {
      return <span style={{color: "red"}}>Failed<a href="javascript:;">  Logs</a></span>;;
    }
  };
  return (
    <div>
      <Table
        dataSource={captureList}
      >
        <Table.Column title="Id" dataIndex="capture_id" sortable />
        <Table.Column
          title="Type"
          dataIndex="capture_type"
        />
        <Table.Column title="Name" dataIndex="capture_name" />
        <Table.Column title="Result" cell={render} width={200} />
      </Table>
    </div>
  );
};

export default CaptureForm;


const captureList = [
  {
    capture_id: 1,
    capture_type: "pod",
    capture_name: "default/nginx",
    task_status: "success",
    files: "kubeskoop_capture_1596880000000_default_nginx_nginx-deployment-6666666666.zip"
  },
  {
    capture_id: 2,
    capture_type: "node",
    capture_name: "cn-hangzhou.192.168.0.2",
    task_status: "failed",
    files: "kubeskoop_capture_1596880000000_cn-hangzhou.192.168.0.2.zip"
  },
  {
    capture_id: 3,
    capture_type: "node",
    capture_name: "cn-hangzhou.192.168.0.2",
    task_status: "running",
    files: "kubeskoop_capture_1596880000000_cn-hangzhou.192.168.0.2.zip"
  },
];
