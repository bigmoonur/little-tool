// 视频缩略图：界面用隐藏的 <video> 解码出一帧，缩小后交给后端缓存；以后直接读缓存。
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from 'react'
import { api } from '../api'
import type { VideoFile } from '../types'

/** ready 可以显示，none 这个视频生成不了，loading 正在排队或生成，idle 还没人要它。 */
export type ThumbState = 'ready' | 'none' | 'loading' | 'idle'

/** 缩略图最长边的像素数。 */
const MAX_SIDE = 400
/** 同时最多解码几个视频。 */
const WORKERS = 2
/** 一个视频最多等这么久，读不出画面就算生成不了。 */
const TIMEOUT = 15000
/** 依次在这几个位置截（占时长的比例）：开头常常是黑场，截到几乎全黑的就往后再试。 */
const SPOTS = [0.1, 0.35, 0.65]

interface Job {
  key: string
  category: string
  name: string
  modified: number
}

/** 这次运行里已经有结果的。后端列表里的 thumb 是打开分类那一刻的状态，这里的更新。 */
const known = new Map<string, 'ready' | 'none'>()
/** key → 有几处正在等它（看得见的行、图标，或者鼠标悬停）。没人等的任务不会开始。 */
const wanted = new Map<string, number>()
const queue: Job[] = []
const queued = new Set<string>()
const working = new Set<string>()
/** 按 key 订阅：一张缩略图好了，只重画用到它的那一处。 */
const listeners = new Map<string, Set<() => void>>()

function notify(key: string) {
  listeners.get(key)?.forEach(fn => fn())
}
function subscribe(key: string, fn: () => void) {
  const set = listeners.get(key) ?? new Set()
  listeners.set(key, set.add(fn))
  return () => {
    set.delete(fn)
    if (!set.size) listeners.delete(key)
  }
}

export const thumbUrl = (key: string) => `/thumbs/${key}.jpg`

function stateOf(v: VideoFile): ThumbState {
  if (!v.key) return 'idle'
  return known.get(v.key) ?? (v.thumb || (queued.has(v.key) || working.has(v.key) ? 'loading' : 'idle'))
}

/** 画面是不是几乎全黑（隔几个像素取一个，算平均亮度）。 */
function tooDark(g: CanvasRenderingContext2D, w: number, h: number) {
  const { data } = g.getImageData(0, 0, w, h)
  let sum = 0
  let n = 0
  for (let i = 0; i < data.length; i += 64) {
    sum += data[i] * 0.299 + data[i + 1] * 0.587 + data[i + 2] * 0.114
    n++
  }
  return sum / n < 18
}

/** 解码出一帧并缩小成 JPEG；读不了（格式不支持、文件坏了、超时）返回 null。 */
function capture(job: Job): Promise<string | null> {
  return new Promise(resolve => {
    const video = document.createElement('video')
    let settled = false
    const finish = (data: string | null) => {
      if (settled) return
      settled = true
      window.clearTimeout(timer)
      video.onloadeddata = video.onseeked = video.onerror = null
      // 放掉这个视频，不再继续下载。
      video.removeAttribute('src')
      video.load()
      resolve(data)
    }
    const timer = window.setTimeout(() => finish(null), TIMEOUT)
    let spot = 0
    const grab = () => {
      const w = video.videoWidth
      const h = video.videoHeight
      if (!w || !h) return finish(null)
      const scale = Math.min(1, MAX_SIDE / Math.max(w, h))
      const canvas = document.createElement('canvas')
      canvas.width = Math.max(1, Math.round(w * scale))
      canvas.height = Math.max(1, Math.round(h * scale))
      try {
        const g = canvas.getContext('2d')!
        g.drawImage(video, 0, 0, canvas.width, canvas.height)
        if (spot < SPOTS.length - 1 && Number.isFinite(video.duration) && tooDark(g, canvas.width, canvas.height)) {
          video.currentTime = video.duration * SPOTS[++spot]
          return
        }
        finish(canvas.toDataURL('image/jpeg', 0.8))
      } catch {
        finish(null)
      }
    }
    video.muted = true
    video.playsInline = true
    video.preload = 'auto'
    video.onerror = () => finish(null)
    video.onseeked = grab
    video.onloadeddata = () => {
      // 第一处最多往后 3 秒，长视频也不用读太远。
      const t = Number.isFinite(video.duration) ? Math.min(video.duration * SPOTS[0], 3) : 0
      if (t < 0.05) return grab()
      video.currentTime = t
    }
    video.src = `/media/${encodeURIComponent(job.category)}/${encodeURIComponent(job.name)}?t=${job.modified}`
  })
}

function pump() {
  while (working.size < WORKERS && queue.length) {
    const job = queue.shift()!
    queued.delete(job.key)
    if (!wanted.get(job.key)) {
      notify(job.key)
      continue
    }
    working.add(job.key)
    capture(job)
      .then(async data => {
        await api.saveThumbnail(job.category, job.name, data ?? '')
        return data ? ('ready' as const) : ('none' as const)
      })
      // 没存上（视频刚被删、磁盘满）：这次先当作没有，下次打开再试。
      .catch(() => 'none' as const)
      .then(result => {
        known.set(job.key, result)
        working.delete(job.key)
        notify(job.key)
        pump()
      })
  }
}

/** 要一张缩略图。urgent（鼠标正悬停着）排到最前面。返回的函数用来说“不要了”。 */
function request(job: Job, urgent: boolean) {
  wanted.set(job.key, (wanted.get(job.key) ?? 0) + 1)
  if (!known.has(job.key) && !working.has(job.key)) {
    const at = queue.findIndex(j => j.key === job.key)
    if (at >= 0 && urgent) queue.unshift(...queue.splice(at, 1))
    else if (at < 0) {
      queued.add(job.key)
      if (urgent) queue.unshift(job)
      else queue.push(job)
      notify(job.key)
    }
    pump()
  }
  return () => {
    const n = (wanted.get(job.key) ?? 1) - 1
    if (n > 0) return void wanted.set(job.key, n)
    wanted.delete(job.key)
    const at = queue.findIndex(j => j.key === job.key)
    if (at >= 0) {
      queue.splice(at, 1)
      queued.delete(job.key)
      notify(job.key)
    }
  }
}

/** 这个视频缩略图的状态。want 为真时（看得见或正悬停）没有就去生成。 */
export function useThumb(category: string, v: VideoFile, want: boolean, urgent = false): ThumbState {
  const state = useSyncExternalStore(
    useCallback((fn: () => void) => subscribe(v.key, fn), [v.key]),
    () => stateOf(v),
  )
  const pending = state === 'idle' || state === 'loading'
  useEffect(() => {
    if (!want || !v.key || !pending) return
    return request({ key: v.key, category, name: v.name, modified: v.modified }, urgent)
  }, [want, urgent, pending, v.key, category, v.name, v.modified])
  return state
}

// ---------- 看得见才生成 ----------

const seen = new Map<Element, (visible: boolean) => void>()
let observer: IntersectionObserver | null = null

function watch(el: Element, fn: (visible: boolean) => void) {
  // 提前一点开始：滚到之前就准备好。
  observer ??= new IntersectionObserver(entries => entries.forEach(e => seen.get(e.target)?.(e.isIntersecting)), { rootMargin: '240px 0px' })
  seen.set(el, fn)
  observer.observe(el)
  return () => {
    seen.delete(el)
    observer?.unobserve(el)
  }
}

/** 元素是否在窗口里（或马上要滚到）。 */
export function useVisible<T extends Element>() {
  const ref = useRef<T>(null)
  const [visible, setVisible] = useState(false)
  useEffect(() => (ref.current ? watch(ref.current, setVisible) : undefined), [])
  return [ref, visible] as const
}
