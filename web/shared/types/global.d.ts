declare global {
  var __workerPool: unknown
  interface Window {
    AMap?: any
    _AMapSecurityConfig?: { securityJsCode?: string }
  }
}

export {}
