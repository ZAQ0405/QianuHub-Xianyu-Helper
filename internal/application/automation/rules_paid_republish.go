package automation

// ensurePaidRepublishConfirmation 为付款后重新上架规则补齐确认发货动作，兼容旧规则和未经过前端组装的调用方。
func ensurePaidRepublishConfirmation(triggerType string, actions []ActionInput, flags ruleActionFlags) ([]ActionInput, ruleActionFlags) {
	if triggerType != TriggerOrderPaid || !flags.hasRepublishItem || flags.hasConfirmShipment {
		return actions, flags
	}
	// nextSortOrder 保存补齐动作使用的下一个排序值，避免覆盖调用方提交的动作顺序。
	nextSortOrder := len(actions) + 1
	// action 表示已规范化的动作，用于处理自定义排序值大于动作数量的规则。
	for _, action := range actions {
		if action.SortOrder >= nextSortOrder {
			nextSortOrder = action.SortOrder + 1
		}
	}
	actions = append(actions, ActionInput{ActionType: ActionConfirmShipment, Enabled: true, ConfigJSON: "{}", SortOrder: nextSortOrder})
	flags.hasConfirmShipment = true
	return actions, flags
}
