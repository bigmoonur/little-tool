import { useRef, useState } from 'react'
import { CircleQuestionMark } from 'lucide-react'
import { REPEAT_MODES } from '../lib/pick'
import type { RepeatMode } from '../types'
import { anchorOf, Popover, type Anchor } from '../ui/Popover'

/** 乘数规则：三段切换，每段带一个小图样；问号里用同一组示范对比三种规则在文件夹里的排列。 */
export function ModeSwitch({ value, onChange, disabled = false }: { value: RepeatMode; onChange: (m: RepeatMode) => void; disabled?: boolean }) {
  const [help, setHelp] = useState<Anchor | null>(null)
  const helpBtn = useRef<HTMLButtonElement>(null)
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
        ref={helpBtn}
        className={`icon-btn sm ${help ? 'is-open' : ''}`}
        aria-label="三种乘数规则有什么不同"
        aria-haspopup="dialog"
        aria-expanded={!!help}
        title={help ? '收起说明' : '三种乘数规则有什么不同'}
        onClick={e => setHelp(help ? null : anchorOf(e.currentTarget, 'end'))}
      >
        <CircleQuestionMark size={16} />
      </button>
      {help && (
        <Popover anchor={help} onClose={() => setHelp(null)} width={452} label="乘数规则说明" className="mode-help" trigger={helpBtn}>
          <div className="pop-title">三种乘数规则</div>
          <p className="mode-help-lead">
            同样填“数量 2、乘数 3”，都会放进去 6 个文件，区别在文件夹里排列的顺序。字母代表抽到的不同视频：
          </p>
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
                <span className="mode-demo-names" aria-label={`文件名示例：${m.names.join('、')}`}>
                  {m.names.map(n => (
                    <span key={n}>{n}</span>
                  ))}
                  <span aria-hidden>…</span>
                </span>
              </button>
            ))}
          </div>
          <p className="mode-help-foot">
            文件夹按名称排序就是上面的顺序，所以靠文件名来排。乘数不填或填 1 时，每个视频只复制一份，文件名不变。
          </p>
        </Popover>
      )}
    </div>
  )
}
