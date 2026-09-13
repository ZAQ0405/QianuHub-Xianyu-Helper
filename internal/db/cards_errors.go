package db

import "errors"

// ErrCardInUse 表示卡券仍被自动化或历史发货规则引用，不能直接删除。
var ErrCardInUse = errors.New("卡券仍被规则引用")
