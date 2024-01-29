import { Card, Button, Message, Icon, Dialog } from "@alifd/next"
import DiagnosisForm from "./components/DiagnosisForm"
import PageHeader from "@/components/PageHeader"
import DiagnosisHistory from "./components/DiagnosisHistory"
import { useEffect, useState } from 'react'
import { DiagnosisResult, DiagnosisTask } from "@/services/diagnosis"
import { Link } from "@ice/runtime"
import diagnosisService from '@/services/diagnosis'
import { getErrorMessage } from "@/utils"
import { definePageConfig } from "ice";


const makeAction = (d: DiagnosisResult, showMessageDialog: (message: string) => void): JSX.Element | null => {
  switch (d.status) {
    case "success":
      return (<span>
        <Button text type="primary" style={{marginRight: '5px'}}><Link className="next-btn-helper" style={{color: 'inherit'}} to={`/diagnosis/result/${d.task_id}`}>Result</Link></Button>
        {d.message ? <Button text type="primary" onClick={(() => showMessageDialog(d.message))}>Log</Button> : null }
      </span>)
    case "failed":
      return d.message ? <Button text onClick={() => showMessageDialog(d.message)}>Log</Button> : null
    default:
      return null;
  }
}

const toTableData = (data: DiagnosisResult[], showMessageDialog: (message: string) => void) => {
  if (!data) return [];
  return data.map(d => {
    const task_config = JSON.parse(d.task_config) as DiagnosisTask;
    return {
      id: d.task_id,
      time: d.start_time,
      src: task_config.source,
      dst: task_config.destination.address,
      port: task_config.destination.port,
      protocol: task_config.protocol,
      status: d.status === 'running' ? <span><Icon size="xs" type="loading" />{d.status}</span> : d.status,
      action: makeAction(d, showMessageDialog)
    }
  })
}

export default function Diagnosis() {
  const [data, setData] = useState<DiagnosisResult[]>([]);;
  const [message, setMessage] = useState('');
  const [dialogVisible, setDialogVisible] = useState(false);
  const [refreshCount, setRefreshCount] = useState(0);

  const refreshDiagnosisList = () => {
    diagnosisService.listDiagnosis()
      .then(res => {
        res = res || []
        setData(res)
      })
      .catch(err => {
        Message.error(`Error fetching diagnosis results: ${getErrorMessage(err)}`)
      }).finally(() => {
        setRefreshCount(refreshCount + 1)
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
        Message.success('Diagnosis task submitted')
        refreshDiagnosisList()
      })
      .catch(err => {
        Message.error(`Error when submitting diagnosis task：${getErrorMessage(err)}`)
      })
  };

  const showMessageDialog = (message) => {
    setMessage(message)
    setDialogVisible(true)
  }

  useEffect(refreshDiagnosisList, [])
  useEffect(() => {
    if (data.find(i => i.status == 'running')) {
      const id = setTimeout(refreshDiagnosisList, 3000);
      return () => clearTimeout(id);
    }
    return () => {}
  }, [refreshCount]);

  return (
    <div>
      <PageHeader
        title='Connectivity Diagnosis'
        breadcrumbs={[{ name: 'Console' }, { name: 'Diagnosis' }, { name: 'Connectivity Diagnosis' }]}
      />
      <Card id="card-diagnosis" title="Diagnose" contentHeight="auto">
        <Card.Content>
          <DiagnosisForm onSubmit={submitDiagnosis} />
        </Card.Content>
      </Card>
      <Card id="card-history" title="History" contentHeight="auto">
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

export const pageConfig = definePageConfig(() => {
  return {
    title: 'Connectivity Diagnosis',
  };
});

