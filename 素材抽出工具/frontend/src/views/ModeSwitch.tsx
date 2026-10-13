import { useState } from 'react'
import { CircleQuestionMark } from 'lucide-react'
import { REPEAT_MODES } from '../lib/pick'
import type { RepeatMode } from '../types'
import { anchorOf, type MenuAnchor } from '../ui/Menu'
import { Popover } from '../ui/Popover'

/** 乘数规则：三段切换，每段带一个小图样；问号里用同一组示范对比三种规则的输出顺序。 */
export function ModeSwitch({ value, onChange }: { value: RepeatMode; onChange: (m: RepeatMode) => void }) {
  const [help, setHelp] = useState<MenuAnchor | null>(null)
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
        className="icon-btn"
        aria-label="三种乘数规则有什么不同"
        aria-haspopup="dialog"
        title="三种乘数规则有什么不同"
        onClick={e => setHelp(anchorOf(e.currentTarget, 'end'))}
      >
        <CircleQuestionMark size={18} />
      </button>
      {help && (
        <Popover anchor={help} onClose={() => setHelp(null)} width={460} label="乘数规则说明" className="mode-help">
          <div className="pop-title">三种乘数规则</div>
          <p className="mode-help-lead">
            同样填“数量 2、乘数 3”，都输出 6 条，区别在排列的顺序。字母代表抽到的不同内容：
          </p>
          <div className="mode-demos" role="radiogroup" aria-label="乘数规则">
            {REPEAT_MODES.map(m => (
              <button
                key={m.mode}
                role="radio"
                aria-checked={value === m.mode}
                className={`mode-demo ${value === m.mode ? 'is-on' : ''}`}
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
          <p className="mode-help-foot">点一种规则就会选中它。乘数不填或填 1 时，每条只输出一次。</p>
        </Popover>
      )}
    </div>
  )
}
