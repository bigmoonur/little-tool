export interface LoadedFile {
  key: string
  fileName: string
  baseName: string
  text: string
  lines: number
  encoding: string
}

const MAX_BYTES = 64 * 1024 * 1024
const TEXT_EXT = /\.(txt|text|csv|tsv|list|md)$/i

export function isTextFile(f: File) {
  return TEXT_EXT.test(f.name) || f.type.startsWith('text/')
}

/** 识别 UTF-8 / UTF-16 / GBK 编码；Windows 记事本旧版默认保存为 GBK。 */
export function decode(buf: Uint8Array): { text: string; encoding: string } {
  if (buf[0] === 0xef && buf[1] === 0xbb && buf[2] === 0xbf) {
    return { text: new TextDecoder('utf-8').decode(buf.subarray(3)), encoding: 'UTF-8' }
  }
  if (buf[0] === 0xff && buf[1] === 0xfe) return { text: new TextDecoder('utf-16le').decode(buf.subarray(2)), encoding: 'UTF-16' }
  if (buf[0] === 0xfe && buf[1] === 0xff) return { text: new TextDecoder('utf-16be').decode(buf.subarray(2)), encoding: 'UTF-16' }
  try {
    return { text: new TextDecoder('utf-8', { fatal: true }).decode(buf), encoding: 'UTF-8' }
  } catch {
    try {
      return { text: new TextDecoder('gb18030').decode(buf), encoding: 'GBK' }
    } catch {
      return { text: new TextDecoder('utf-8').decode(buf), encoding: 'UTF-8' }
    }
  }
}

export function countLines(text: string) {
  let n = 0
  for (const line of text.split('\n')) if (line.trim() !== '') n++
  return n
}

let seq = 0

export async function loadFiles(files: File[]): Promise<{ loaded: LoadedFile[]; skipped: string[] }> {
  const loaded: LoadedFile[] = []
  const skipped: string[] = []
  for (const f of files) {
    if (!isTextFile(f) || f.size > MAX_BYTES) {
      skipped.push(f.name)
      continue
    }
    const { text, encoding } = decode(new Uint8Array(await f.arrayBuffer()))
    const baseName = f.name.replace(/\.[^.]+$/, '').trim().slice(0, 40) || '未命名'
    loaded.push({ key: `f${++seq}`, fileName: f.name, baseName, text, lines: countLines(text), encoding })
  }
  return { loaded, skipped }
}

/** 打开系统文件选择框选择 TXT。 */
export function pickFiles(): Promise<File[]> {
  return new Promise(resolve => {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    input.accept = '.txt,.csv,.tsv,.md,text/plain'
    input.onchange = () => resolve(Array.from(input.files ?? []))
    input.click()
  })
}

export function hasFiles(e: DragEvent | React.DragEvent) {
  return Array.from(e.dataTransfer?.types ?? []).includes('Files')
}
