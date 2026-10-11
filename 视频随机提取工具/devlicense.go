//go:build devlicense && !production

package main

// 只用于本地开发，production 构建即使误传 devlicense 也不能免授权。
const licenseBypass = true
