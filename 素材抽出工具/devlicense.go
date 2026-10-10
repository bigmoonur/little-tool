//go:build devlicense

package main

// 开发时跳过授权检查。发布构建不带 devlicense 标记，这个文件不会被编译进去。
func init() { licenseBypass = true }
