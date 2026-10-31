declare global {
  var __workerPool: unknown
  interface Window {
    AMap?: any
    _AMapSecurityConfig?: { securityJsCode?: string }
    // Tencent's GL API and Tianditu's 4.0 API arrive as globals from a script
    // tag and have no type packages of their own.
    TMap?: any
    T?: any
  }
}

export {}
