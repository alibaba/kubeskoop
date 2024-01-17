import { Card, Button, Table, Message } from "@alifd/next"
import PageHeader from "@/components/PageHeader"
import CaptureForm from "@/pages/capture/components/captureForm";
import CaptureResult from "@/pages/capture/components/captureResult";
import { CaptureTask } from "@/services/capture";
import captureService from "@/services/capture"
import { useEffect, useState } from "react";
import { getErrorMessage } from "@/utils";

const submitCapture = (props, callback) => {
  const task: CaptureTask = {
    capture_list: props.capture_list,
    capture_host_ns: props.capture_node,
    capture_duration_seconds: props.duration,
    filter: props.filter
  }

  captureService.createCapture(task)
    .then(res => {
      Message.success('Capture task submitted.')
      callback()
    })
    .catch(err => {
      Message.error(`Error when submitting capture task: ${getErrorMessage(err)}`)
    })
}

export default function Capture() {
  const [captureList, setCaptureList] = useState([])
  const [abortController, setAbortController] = useState<AbortController | null>(null);
  const [refreshCount, setRefreshCount] = useState(0);
  const refreshCaptureList = () => {
    if (abortController) {
      abortController.abort()
    }
    const c = new AbortController();
    const { signal } = c;
    setAbortController(c);
    captureService.listCaptures(signal)
      .then(res => {
        if (res == null) {
          res = []
        }
        setCaptureList(Object.values(res))
      })
      .catch(err => {
        Message.error(`Error when fetching diagnosis: ${getErrorMessage(err)}`)
      })
      .finally(() => setRefreshCount(refreshCount + 1))
  }
  useEffect(refreshCaptureList, [])
  useEffect(() => {
    if (captureList.flat().find(i => i.status === 'running')) {
      const id = setTimeout(refreshCaptureList, 3000)
      return () => clearTimeout(id)
    }
    return () => {}
  }, [refreshCount]);

  return (
    <div>
      <PageHeader
        title='Packet Capturing'
        breadcrumbs={[{ name: 'Console' }, { name: 'Diagnosis' }, { name: 'Packet Capturing' }]}
      />
      <Card id="card-capture" title="Capture" contentHeight="auto">
        <Card.Content>
          <CaptureForm onSubmit={(props) => submitCapture(props, refreshCaptureList)} />
        </Card.Content>
      </Card>
      <Card id="card-capture-tasks" title="History" contentHeight="auto">
        <Card.Content>
          <CaptureResult captureResult={captureList} />
        </Card.Content>
      </Card>
    </div>
  )
}
