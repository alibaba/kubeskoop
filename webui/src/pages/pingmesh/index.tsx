import {Card, Button, Table, Message} from "@alifd/next"
import PageHeader from "@/components/PageHeader"
import {useEffect, useState} from "react";
import PingForm from "@/pages/pingmesh/pingForm";
import PingGraph from "@/pages/pingmesh/pingGraph";
import pingMeshService from "@/services/pingmesh";
import {getErrorMessage} from "@/utils";

export default function Capture() {
    const [latency, setLatency] = useState()

    return (
        <div>
          <PageHeader
          title='网络延迟探测'
          breadcrumbs={[{name: 'Console'}, {name: '延迟探测'}]}
          />
          <Card id="card-capture" title="延迟探测(PingMesh)" contentHeight="auto">
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
          <Card id="card-capture-tasks" title="探测结果" contentHeight="auto">
            <Card.Content>
              {latency && <PingGraph data={latency}/>}
            </Card.Content>
          </Card>
        </div>
    )
}
