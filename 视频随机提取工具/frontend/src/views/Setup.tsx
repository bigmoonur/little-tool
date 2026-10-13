import { Check, FolderOpen } from 'lucide-react'
import { api } from '../api'
import { useApp } from '../ctx'
import { Logo } from '../ui/Logo'

/** 第一次使用：依次选择素材根目录和目标根目录。 */
export function Setup() {
  const { state, setState, run } = useApp()

  const chooseMaterial = async () => {
    const s = await run(api.chooseMaterialRoot())
    if (s) setState(s)
  }
  const chooseOutput = async () => {
    const s = await run(api.chooseOutputRoot())
    if (s) setState(s)
  }

  return (
    <div className="setup">
      <div className="setup-card">
        <Logo className="setup-logo" />
        <h1>先选两个文件夹</h1>
        <ol className="setup-steps">
          <li className={state.materialRoot ? 'is-done' : ''}>
            <span className="step-num">{state.materialRoot ? <Check size={15} /> : 1}</span>
            <div className="step-body">
              <h2>素材根目录</h2>
              <p>里面的每个子文件夹是一个分类，视频直接放在分类文件夹里。</p>
              <div className="folder-sample" aria-hidden>
                <span className="fs-root">素材根目录</span>
                <span className="fs-cat">开场</span>
                <span className="fs-file">视频1.mp4</span>
                <span className="fs-cat">产品展示</span>
                <span className="fs-cat">结尾</span>
              </div>
              {state.materialRoot ? (
                <div className="step-path">
                  <span title={state.materialRoot}>{state.materialRoot}</span>
                  <button className="link" onClick={chooseMaterial}>
                    更换
                  </button>
                </div>
              ) : (
                <button className="btn btn-primary" onClick={chooseMaterial}>
                  <FolderOpen size={16} />
                  选择素材根目录
                </button>
              )}
            </div>
          </li>
          <li className={state.outputRoot ? 'is-done' : ''}>
            <span className="step-num">{state.outputRoot ? <Check size={15} /> : 2}</span>
            <div className="step-body">
              <h2>目标根目录</h2>
              <p>抽到的视频会放进它下面的一个文件夹，之后可以在主界面选择或新建。它和素材根目录必须是两个独立的文件夹。</p>
              {state.outputRoot ? (
                <div className="step-path">
                  <span title={state.outputRoot}>{state.outputRoot}</span>
                  <button className="link" onClick={chooseOutput}>
                    更换
                  </button>
                </div>
              ) : (
                <button className={`btn ${state.materialRoot ? 'btn-primary' : ''}`} onClick={chooseOutput}>
                  <FolderOpen size={16} />
                  选择目标根目录
                </button>
              )}
            </div>
          </li>
        </ol>
      </div>
    </div>
  )
}
