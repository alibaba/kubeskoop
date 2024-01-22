import {Card, Message} from "@alifd/next"
import PageHeader from "@/components/PageHeader"
import {useState} from "react";
import PingForm from "@/pages/pingmesh/pingForm";
import PingGraph from "@/pages/pingmesh/pingGraph";
import pingMeshService from "@/services/pingmesh";
import {getErrorMessage} from "@/utils";

export default function Capture() {
    const [latency, setLatency] = useState()

    return (
        <div>
          <PageHeader
          title='Latency Detection(PingMesh)'
          breadcrumbs={[{name: 'Console'}, {name: 'Latency Detection'}]}
          />
          <Card id="card-capture" title="Detect" contentHeight="auto">
              <Card.Content>
              <PingForm onSubmit={(values) => {
                  pingMeshService.pingMeshLatency(values).then(res => {
                    setLatency(res)
                  }).catch(err => {
                    Message.error(`error get ping mesh result：${getErrorMessage(err)}`)
                  })
              }}/>
              </Card.Content>
          </Card>
          <Card id="card-capture-tasks" title="Result" contentHeight="auto">
            <Card.Content>
              {latency && <PingGraph data={latency}/>}
            </Card.Content>
          </Card>
        </div>
    )
}
