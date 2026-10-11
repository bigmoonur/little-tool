import { Component, type ReactNode } from 'react'

/** 界面出错时显示说明和“重新加载”，而不是整页空白。 */
export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error) {
    console.error(error)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="empty">
        <div className="empty-card">
          <h2>这个页面出错了</h2>
          <p className="empty-lead">文件和设置不受影响。可以切换到其他页面，或重新加载界面。</p>
          <p className="dim crash-detail">{this.state.error.message}</p>
          <button className="btn btn-primary btn-lg" onClick={() => location.reload()}>
            重新加载
          </button>
        </div>
      </div>
    )
  }
}
