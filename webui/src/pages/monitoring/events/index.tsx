import PageHeader from "@/components/PageHeader"
import WebFrame from "@/components/WebFrameCard"

export default function Events() {
    return (
        <div>
            <PageHeader
            title='事件'
            breadcrumbs = {[{name: 'Console'}, {name: '监控'}, {name: '事件'}]}
            />
            <WebFrame src="/diagnosis" />
        </div>
    )
}
