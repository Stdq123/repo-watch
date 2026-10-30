package main

import "errors"

func fmtNotificationUnsupported() error { return errors.New("当前系统不支持桌面通知") }
