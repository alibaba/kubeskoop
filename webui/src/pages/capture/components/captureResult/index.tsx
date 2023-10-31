import {Button, Table, Icon, Message} from '@alifd/next';
import {useEffect, useState} from "react";
import moment from 'moment';
import {CaptureResult} from "@/services/capture";
import captureService from "@/services/capture";
import {requestConfig} from "@/app";


const convertToTable = (res)=> {
  return res.map((i: CaptureResult) => {
      return {
        capture_id: i.task_id,
        capture_type: i.task_config.pod.name == "" ? "Node": "Pods",
        capture_name: i.task_config.pod.name == "" ? i.task_config.node.name: i.task_config.pod.namespace+"/"+i.task_config.pod.name,
        task_status: i.status,
        message: i.message,
      }
    })
}

interface CaptureTableProps {
  captureResult: CaptureResult[];
}

const CaptureHistory: React.FunctionComponent<CaptureTableProps> = (props: CaptureTableProps) => {
  const render = (value, index, record) => {
    if (record.task_status === "success") {
      return <a href={requestConfig.baseURL+"/controller/capture/"+record.capture_id+"/download"} target="_blank">Download</a>;
    } else if (record.task_status === "running") {
      return <span style={{color: "green"}}>Running</span>;
    } else {
      return <span style={{color: "red"}}>Failed {record.message}</span>;;
    }
  };
  return (
    <div>
      <Table
        dataSource={convertToTable(props.captureResult)}
      >
        <Table.Column title="Id" dataIndex="capture_id" sortable />
        <Table.Column
          title="Type"
          dataIndex="capture_type"
        />
        <Table.Column title="Name" dataIndex="capture_name" />
        <Table.Column title="Status" dataIndex="task_status" />
        <Table.Column title="Result" cell={render} width={200} />
      </Table>
    </div>
  );
};

export default CaptureHistory;
