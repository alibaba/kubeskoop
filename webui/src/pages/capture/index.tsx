import {Card, Button, Table} from "@alifd/next"
import PageHeader from "@/components/PageHeader"
import CaptureForm from "@/pages/capture/components/captureForm";
import CaptureResult from "@/pages/capture/components/captureResult";

const testData = [
{
    id: "1",
    time: "2023-01-05 12:00:00",
    src: "123.123.123.123",
    dst: "10.10.10.10",
    port: 80,
    protocol: "tcp",
    status: "succeed",
    action: <Button text>Hello</Button>
}
]
export default function Diagnosis() {
    return (
        <div>
          <PageHeader
          title='网络抓包'
          breadcrumbs={[{name: 'Console'}, {name: '抓包'}, {name: '分布式抓包'}]}
          />
          <Card id="card-capture" title="抓包" contentHeight="auto">
              <Card.Content>
                  <CaptureForm onSubmit={(props) => console.log(props)} />
              </Card.Content>
          </Card>
          <Card id="card-capture-tasks" title="抓包任务" contentHeight="auto">
            <Card.Content>
              <CaptureResult/>
            </Card.Content>
          </Card>
        </div>
    )
}
