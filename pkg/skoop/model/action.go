package model

type ActionType string

const (
	ActionTypeService = "service"
	ActionTypeServe   = "serve"
	ActionTypeForward = "forward"
	ActionTypeSend    = "send"
)

type Action struct {
	Type    ActionType
	Input   *Link
	Outputs []*Link
}

func ActionForward(input *Link, outputs []*Link) *Action {
	return &Action{
		Type:    ActionTypeForward,
		Input:   input,
		Outputs: outputs,
	}
}

func ActionSend(outputs []*Link) *Action {
	return &Action{
		Type:    ActionTypeSend,
		Outputs: outputs,
	}
}

func ActionServe(input *Link) *Action {
	return &Action{
		Type:  ActionTypeServe,
		Input: input,
	}
}

func ActionService(input *Link, outputs []*Link) *Action {
	return &Action{
		Type:    ActionTypeService,
		Input:   input,
		Outputs: outputs,
	}
}

// we use Type to replace these action structs
//type Service struct {
//	*Action
//}
//
//type Serve struct {
//	*Action
//}
//
//type Forward struct {
//	*Action
//}
//
//type Send struct {
//	*Action
//}
