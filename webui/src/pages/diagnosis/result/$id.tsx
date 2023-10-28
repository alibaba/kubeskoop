import { useParams } from "ice";
import { Card, Message } from "@alifd/next";
import DiagnosisGraph from "./components/DiagnosisGraph";
import PageHeader from "@/components/PageHeader";
import ResultDialog from "./components/ResultDialog";
import { useEffect, useState } from "react";
import { ResultDialogData } from "./components/ResultDialog";
import diagnosisService from "@/services/diagnosis";

export default function Result() {
	const params = useParams()
	const [visible, setVisible] = useState(false)
	const [dialogData, setDialogData] = useState<ResultDialogData>()
	const [data, setData] = useState<DiagnosisResultData>();
	useEffect(() => {
		diagnosisService.getDiagnosisById(params.id)
		.then(res => {
			setData(JSON.parse(res.result))
		})
		.catch(err => {
			Message.error(`Error when getting diagnosis result: ${err.message}`)
		})
	}, [])


	const showResultDialog = (type, id) => {
		let newDialogData: DiagnosisNode | DiagnosisLink | undefined;
		switch(type) {
			case "node":
				newDialogData = data?.nodes?.find(item => item.id === id)
				break;
			case "edge":
				newDialogData = data?.links?.find(item => item.id === id)
				break;
		}

		if (newDialogData) {
			setDialogData({
				"type": type,
				"data": newDialogData
			})
			setVisible(true)
		}
	};

	return (
		<div>
			<PageHeader
				title="诊断结果"
				breadcrumbs={[{ name: 'Console' }, { name: '诊断' }, { name: '连通性诊断',  path: '/diagnosis'}, { name: '诊断结果' }]}
			/>
			<Card title="链路图" contentHeight="auto">
				<Card.Content>
					<DiagnosisGraph data={data} onClick={showResultDialog}/>
				</Card.Content>
			</Card>
		<ResultDialog data={dialogData} visible={visible} onClose={() => setVisible(false)}/>
		</div>
	)
}
