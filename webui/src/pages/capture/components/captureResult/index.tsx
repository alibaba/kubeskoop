import {Button, Table, Icon, Message} from '@alifd/next';
import {useEffect, useState} from "react";
import moment from 'moment';
import {CaptureResult} from "@/services/capture";
import captureService from "@/services/capture";
import {requestConfig} from "@/app";


const convertToTable = (res)=> {
  return res.map((i: CaptureResult[]) => {
      return {
        capture_id: i[0].task_id,
        capture_names: i.map((capture) => capture.spec.task_type+": "+capture.spec.name).join(", "),
        capture_results: i
      }
    })
}

interface CaptureTableProps {
  captureResult: CaptureResult[][];
}

const CaptureHistory: React.FunctionComponent<CaptureTableProps> = (props: CaptureTableProps) => {
  const render = (value, index, record) => {
    if (record.capture_results.reduce((prev, item)=> {
      return prev && item.status==="success"},true)) {
      return <a href={requestConfig.baseURL+"/controller/capture/"+record.capture_id+"/download"} target="_blank">Download</a>;
    } else if (record.capture_results.reduce((prev, item)=>{return prev || item.status==="running"}, false)) {
      return <span style={{color: "green"}}>Running</span>;
    } else {
      return <div style={{color: "red"}}>Failed {record.capture_results.map(item => item.message).join(",")}</div>
    }
  };
  return (
    <div>
      <Table
        dataSource={convertToTable(props.captureResult.filter((i)=>i!=null))}
      >
        <Table.Column title="Id" dataIndex="capture_id" sortable />
        <Table.Column title="CaptureObjects" dataIndex="capture_names" />
        <Table.Column title="Result" cell={render} width={200} />
      </Table>
    </div>
  );
};

export default CaptureHistory;
