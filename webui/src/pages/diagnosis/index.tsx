import { Card, Button, Message } from "@alifd/next"
import DiagnosisForm from "./components/DiagnosisForm"
import PageHeader from "@/components/PageHeader"
import DiagnosisHistory from "./components/DiagnosisHistory"
import { useEffect, useState } from 'react'
import { DiagnosisResult, DiagnosisTask } from "@/services/diagnosis"
import { Link } from "@ice/runtime"
import diagnosisService from '@/services/diagnosis'


const makeAction = (d: DiagnosisResult): JSX.Element | null => {
  switch (d.status) {
    case "success":
      return <Button text><Link to={`/diagnosis/result/${d.task_id}`}>查看结果</Link></Button>
    case "failed":
      return <Button text>查看错误</Button>
    default:
      return null;
  }
}

const toTableData = (data: DiagnosisResult[]) => {
  return data.map(d => {
    return {
      id: d.task_id,
      time: d.start_time,
      src: d.task_config.source,
      dst: d.task_config.destination.address,
      port: d.task_config.destination.port,
      protocol: d.task_config.protocol,
      status: d.status,
      action: makeAction(d)
    }
  })
}

export default function Diagnosis() {
  const [data, setData] = useState<DiagnosisResult[]>([])

  const refreshDiagnosisList = () => {
    diagnosisService.listDiagnosis()
    .then(res => {
      setData(res)
      if (res.find(i => i.status == 'running')) setTimeout(refreshDiagnosisList, 3000)
    })
    .catch(err => {
      Message.error(`Error when fetching diagnosis: ${err.response.data.error}`)
    })
  }

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
      Message.error(`Error when submitting diagnosis: ${err.response.data.error}`)
    })
  }

  useEffect(refreshDiagnosisList, [])

  return (
    <div>
      <PageHeader
      title='连通性诊断'
      breadcrumbs={[{name: 'Console'}, {name: '诊断'}, {name: '连通性诊断'}]}
      />
      <Card id="card-diagnosis" title="诊断" contentHeight="auto">
        <Card.Content>
          <DiagnosisForm onSubmit={submitDiagnosis}/>
        </Card.Content>
      </Card>
      <Card id="card-history" title="诊断历史" contentHeight="auto">
        <Card.Content>
          <DiagnosisHistory data={toTableData(data)}/>
        </Card.Content>
      </Card>
    </div>
  )
}
