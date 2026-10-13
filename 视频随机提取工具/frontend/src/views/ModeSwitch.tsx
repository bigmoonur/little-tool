import { useState } from 'react'
import { CircleQuestionMark } from 'lucide-react'
import { REPEAT_MODES } from '../lib/pick'
import type { RepeatMode } from '../types'
import { anchorOf, Popover, type Anchor } from '../ui/Popover'

/** 乘数规则：两段切换，每段带一个小图样；问号里用同一组示范对比两种规则。 */
export function ModeSwitch({ value, onChange, disabled = false }: { value: RepeatMode; onChange: (m: RepeatMode) => void; disabled?: boolean }) {
  const [help, setHelp] = useState<Anchor | null>(null)
  return (
    <div className="mode-switch">
      <span className="mode-switch-label" id="mode-label">
        乘数规则
      </span>
      <div className="seg" role="radiogroup" aria-labelledby="mode-label">
        {REPEAT_MODES.map(m => (
          <button
            key={m.mode}
            role="radio"
            aria-checked={value === m.mode}
            aria-label={m.label}
            className={`seg-btn ${value === m.mode ? 'is-on' : ''}`}
            title={`以“数量 2、乘数 3”为例：${m.help}`}
            disabled={disabled}
            onClick={() => onChange(m.mode)}
          >
            <span className="mode-glyph" aria-hidden>
              {[...m.glyph].map((c, i) => (
                <i key={i} className={`t-${c}`} />
              ))}
            </span>
            <span className="seg-full">{m.label}</span>
            <span className="seg-short">{m.short}</span>
          </button>
        ))}
      </div>
      <button
        className="icon-btn sm"
        aria-label="两种乘数规则有什么不同"
        aria-haspopup="dialog"
        title="两种乘数规则有什么不同"
        onClick={e => setHelp(anchorOf(e.currentTarget, 'end'))}
      >
        <CircleQuestionMark size={16} />
      </button>
      {help && (
        <Popover anchor={help} onClose={() => setHelp(null)} width={420} label="乘数规则说明" className="mode-help">
          <div className="pop-title">两种乘数规则</div>
          <p className="mode-help-lead">同样填“数量 2、乘数 3”，都会放进去 6 个文件。字母代表抽到的不同视频：</p>
          <div className="mode-demos" role="radiogroup" aria-label="乘数规则">
            {REPEAT_MODES.map(m => (
              <button
                key={m.mode}
                role="radio"
                aria-checked={value === m.mode}
                className={`mode-demo ${value === m.mode ? 'is-on' : ''}`}
                disabled={disabled}
                onClick={() => onChange(m.mode)}
              >
                <span className="mode-demo-name">{m.label}</span>
                <span className="mode-demo-tiles" aria-hidden>
                  {m.demo.map((group, i) => (
                    <span className="tile-group" key={i}>
                      {group.map((c, j) => (
                        <i key={j} className={`tile t-${c}`}>
                          {c}
                        </i>
                      ))}
                    </span>
                  ))}
                </span>
                <span className="mode-demo-help">{m.help}</span>
              </button>
            ))}
          </div>
          <p className="mode-help-foot">同一个视频的第一份用原来的文件名，其余的副本会在文件名后面加上随机字符，例如“开场1_k.mp4”。乘数不填或填 1 时，每个视频只复制一份。</p>
        </Popover>
      )}
    </div>
  )
}
