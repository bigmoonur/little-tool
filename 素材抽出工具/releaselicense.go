//go:build !devlicense || production

package main

// 编译期常量使正式版中不包含可在运行时切换的免授权分支。
const licenseBypass = false
