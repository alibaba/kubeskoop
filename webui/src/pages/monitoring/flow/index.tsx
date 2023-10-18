import PageHeader from "@/components/PageHeader"
import WebFrame from "@/components/WebFrameCard"

export default function Events() {
    return (
        <div>
            <PageHeader
            title='flow'
            breadcrumbs = {[{name: 'Console'}, {name: '监控'}, {name: 'flow'}]}
            />
            <WebFrame src="/diagnosis" />
        </div>
    )
}
