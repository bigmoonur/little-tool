/** 按 Windows 的规则检查文件夹名称，与后端 picker.ValidateFolderName 一致；通过时返回空字符串。 */
export function validateFolderName(name: string) {
  if (!name.trim()) return '请输入文件夹名称'
  if (name === '.' || name === '..' || /[ .]$/.test(name)) return '名称不能是 . 或 ..，也不能以空格或句点结尾'
  if (/[<>:"/\\|?*\x00-\x1f]/.test(name)) return '名称不能包含以下字符：< > : " / \\ | ? *'
  const stem = name.split('.')[0].trimEnd().toUpperCase()
  if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(stem)) return '这个名称是 Windows 保留名称，请换一个'
  return ''
}
