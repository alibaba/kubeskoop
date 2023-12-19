import {Card, Button, Table, Message} from "@alifd/next"
import PageHeader from "@/components/PageHeader"
import {useEffect, useState} from "react";
import PingForm from "@/pages/pingmesh/pingForm";

export default function Capture() {
    return (
        <div>
          <PageHeader
          title='网络延迟探测'
          breadcrumbs={[{name: 'Console'}, {name: '延迟探测'}]}
          />
          <Card id="card-capture" title="延迟探测(PingMesh)" contentHeight="auto">
              <Card.Content>
              <PingForm onSubmit={(values) => {
                  console.log(values)
                  Message.success('提交成功')
              }}/>
              </Card.Content>
          </Card>
          <Card id="card-capture-tasks" title="探测结果" contentHeight="auto">
            <Card.Content>

            </Card.Content>
          </Card>
        </div>
    )
}
