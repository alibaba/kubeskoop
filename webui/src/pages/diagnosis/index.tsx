import { Card, Button } from "@alifd/next"
import DiagnosisForm from "./components/DiagnosisForm"
import PageHeader from "@/components/PageHeader"
import DiagnosisHistory from "./components/DiagnosisHistory"

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
            title='连通性诊断'
            breadcrumbs={[{name: 'Console'}, {name: '诊断'}, {name: '连通性诊断'}]}
            />
            <Card id="card-diagnosis" title="诊断" contentHeight="auto">
                <Card.Content>
                    <DiagnosisForm onSubmit={(props) => console.log(props)}/>
                </Card.Content>
            </Card>
            <Card id="card-history" title="诊断历史" contentHeight="auto">
                <Card.Content>
                    <DiagnosisHistory data={testData}/>
                </Card.Content>
            </Card>
        </div>
    )
}
