import { Card, Button, Message, Icon, Dialog } from "@alifd/next"
import DiagnosisForm from "./components/DiagnosisForm"
import PageHeader from "@/components/PageHeader"
import DiagnosisHistory from "./components/DiagnosisHistory"
import { useEffect, useState } from 'react'
import { DiagnosisResult, DiagnosisTask } from "@/services/diagnosis"
import { Link } from "@ice/runtime"
import diagnosisService from '@/services/diagnosis'
import { getErrorMessage } from "@/utils"


const makeAction = (d: DiagnosisResult, showMessageDialog: (message: string) => void): JSX.Element | null => {
  switch (d.status) {
    case "success":
      return (<span>
        <Button text type="primary" style={{marginRight: '5px'}}><Link className="next-btn-helper" style={{color: 'inherit'}} to={`/diagnosis/result/${d.task_id}`}>查看结果</Link></Button>
        {d.message ? <Button text type="primary" onClick={(() => showMessageDialog(d.message))}>查看日志</Button> : null }
      </span>)
    case "failed":
      return d.message ? <Button text onClick={() => showMessageDialog(d.message)}>查看日志</Button> : null
    default:
      return null;
  }
}

const toTableData = (data: DiagnosisResult[], showMessageDialog: (message: string) => void) => {
  if (!data) return [];
  return data.map(d => {
    return {
      id: d.task_id,
      time: d.start_time,
      src: d.task_config.source,
      dst: d.task_config.destination.address,
      port: d.task_config.destination.port,
      protocol: d.task_config.protocol,
      status: d.status === 'running' ? <span><Icon size="xs" type="loading" />{d.status}</span> : d.status,
      action: makeAction(d, showMessageDialog)
    }
  })
}

export default function Diagnosis() {
  const [data, setData] = useState<DiagnosisResult[]>([]);;
  const [message, setMessage] = useState('');
  const [dialogVisible, setDialogVisible] = useState(false);

  const refreshDiagnosisList = () => {
    diagnosisService.listDiagnosis()
      .then(res => {
        res = res || []
        setData(res)
        if (res.find(i => i.status == 'running')) setTimeout(refreshDiagnosisList, 3000)
      })
      .catch(err => {
        Message.error(`获取诊断信息失败： ${getErrorMessage(err)}`)
      })
  };

  const submitDiagnosis = (props) => {
    const task: DiagnosisTask = {
      source: props.src,
      destination: {
        address: props.dst,
        port: props.port
      },
      protocol: props.protocol
    };
    diagnosisService.createDiagnosis(task)
      .then(res => {
        Message.success('诊断提交成功')
        refreshDiagnosisList()
      })
      .catch(err => {
        Message.error(`诊断提交失败：${getErrorMessage(err)}`)
      })
  };

  const showMessageDialog = (message) => {
    setMessage(message)
    setDialogVisible(true)
  }

  useEffect(refreshDiagnosisList, [])

  return (
    <div>
      <PageHeader
        title='连通性诊断'
        breadcrumbs={[{ name: 'Console' }, { name: '诊断' }, { name: '连通性诊断' }]}
      />
      <Card id="card-diagnosis" title="诊断" contentHeight="auto">
        <Card.Content>
          <DiagnosisForm onSubmit={submitDiagnosis} />
        </Card.Content>
      </Card>
      <Card id="card-history" title="诊断历史" contentHeight="auto">
        <Card.Content>
          <DiagnosisHistory data={toTableData(data, showMessageDialog)} />
        </Card.Content>
      </Card>
      <Dialog
        v2
        visible={dialogVisible}
        footerActions={['ok']}
        onOk={() => setDialogVisible(false)}
        onClose={() => setDialogVisible(false)}>
          {message}
      </Dialog>
    </div>
  );
}
