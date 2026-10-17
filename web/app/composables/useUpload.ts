import type { UploadPlan } from '~~/shared/types/upload'
import type { UploadTransportStatus } from '~~/shared/utils/upload-status'

export interface UploadProgress {
  loaded: number
  total: number
  percentage: number
  speed?: number // bytes per second
  timeRemaining?: number // seconds
}

export interface UploadStatus {
  status: UploadTransportStatus
  progress: UploadProgress
  error?: string
  startTime?: number
  endTime?: number
}

export interface UploadCallbacks {
  onProgress?: (progress: UploadProgress) => void
  onStatusChange?: (status: UploadTransportStatus) => void
  onSuccess?: (response?: XMLHttpRequest) => void
  onError?: (error: string, xhr?: XMLHttpRequest) => void
  onAbort?: () => void
  onRetry?: (attempt: number, maxAttempts: number) => void
}

export interface UseUploadOptions {
  timeout?: number // 超时时间（毫秒）
  withCredentials?: boolean
  headers?: Record<string, string>
  speedSampleSize?: number // 用于计算速度的样本数量
  maxRetries?: number // 最大重试次数
  retryDelay?: number // 重试延迟（毫秒）
}

function responseErrorMessage(xhr: XMLHttpRequest, fallback: string): string {
  const responseText = xhr.responseText?.trim()
  if (!responseText) return fallback
  try {
    const response = JSON.parse(responseText) as {
      message?: unknown
      statusMessage?: unknown
      data?: { message?: unknown; title?: unknown }
    }
    const message =
      response.message ??
      response.statusMessage ??
      response.data?.message ??
      response.data?.title
    return typeof message === 'string' && message.trim()
      ? message.trim()
      : fallback
  } catch {
    return responseText
  }
}

export function useUpload(options: UseUploadOptions = {}) {
  // useUpload() is invoked from within an async upload handler (outside the
  // synchronous setup context), so useI18n() would throw "Must be called at
  // the top of a `setup` function". Use the Nuxt-global i18n instead, which is
  // safe to access outside setup (see useExifLocalization).
  const { $i18n } = useNuxtApp()
  const t = $i18n.t
  const {
    timeout = 0,
    withCredentials = false,
    headers = {},
    speedSampleSize = 5,
    maxRetries = 3,
    retryDelay = 1000,
  } = options

  // 响应式状态
  const uploadStatus = ref<UploadStatus>({
    status: 'idle',
    progress: {
      loaded: 0,
      total: 0,
      percentage: 0,
    },
  })

  // 当前的 XMLHttpRequest 实例
  let currentXHR: XMLHttpRequest | null = null

  // The requests of an upload sent in parts: the parts in flight, and whether
  // the upload should stop (the person gave up) or has already failed.
  const partXHRs = new Set<XMLHttpRequest>()
  let partsRunning = false
  let cancelled = false
  let halted = false

  // 用于计算速度的数据点
  const speedSamples: Array<{ timestamp: number; loaded: number }> = []

  // 计算上传速度和剩余时间
  const calculateSpeed = (
    loaded: number,
  ): { speed: number; timeRemaining?: number } => {
    const now = Date.now()
    speedSamples.push({ timestamp: now, loaded })

    // 保持样本数量在限制内
    if (speedSamples.length > speedSampleSize) {
      speedSamples.shift()
    }

    if (speedSamples.length < 2) {
      return { speed: 0 }
    }

    // 计算平均速度
    const firstSample = speedSamples[0]
    const lastSample = speedSamples[speedSamples.length - 1]

    if (!firstSample || !lastSample) {
      return { speed: 0 }
    }

    const timeDiff = (lastSample.timestamp - firstSample.timestamp) / 1000 // 转换为秒
    const bytesDiff = lastSample.loaded - firstSample.loaded

    const speed = timeDiff > 0 ? bytesDiff / timeDiff : 0

    // 计算剩余时间
    const total = uploadStatus.value.progress.total
    const remaining = total - loaded
    const timeRemaining = speed > 0 ? remaining / speed : undefined

    return { speed, timeRemaining }
  }

  // 更新状态
  const updateStatus = (updates: Partial<UploadStatus>) => {
    uploadStatus.value = { ...uploadStatus.value, ...updates }
  }

  // 更新进度
  const updateProgress = (loaded: number, total: number) => {
    const percentage = total > 0 ? Math.round((loaded / total) * 100) : 0
    const { speed, timeRemaining } = calculateSpeed(loaded)

    const progress: UploadProgress = {
      loaded,
      total,
      percentage,
      speed,
      timeRemaining,
    }

    updateStatus({ progress })
  }

  // 重置状态
  const resetStatus = () => {
    speedSamples.length = 0
    updateStatus({
      status: 'idle',
      progress: {
        loaded: 0,
        total: 0,
        percentage: 0,
      },
      error: undefined,
      startTime: undefined,
      endTime: undefined,
    })
  }

  // 主要的上传函数：单次请求发送整个文件
  const uploadAlone = async (
    file: File,
    signedUrl: string,
    callbacks: UploadCallbacks = {},
    attempt: number = 1,
  ): Promise<XMLHttpRequest> => {
    // 重置状态（仅在第一次尝试时）
    if (attempt === 1) {
      resetStatus()
      cancelled = false
      halted = false
    }

    return new Promise((resolve, reject) => {
      // 创建新的 XHR 实例
      currentXHR = new XMLHttpRequest()
      const xhr = currentXHR

      // 设置超时
      if (timeout > 0) {
        xhr.timeout = timeout
      }

      // 设置是否携带凭证
      xhr.withCredentials = withCredentials

      // 记录开始时间
      const startTime = Date.now()
      updateStatus({ status: 'uploading', startTime })
      callbacks.onStatusChange?.('uploading')

      // 进度事件处理
      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
          updateProgress(event.loaded, event.total)
          callbacks.onProgress?.(uploadStatus.value.progress)
        }
      })

      // 成功处理
      xhr.addEventListener('load', () => {
        if (xhr.status < 200 || xhr.status >= 300) return
        const endTime = Date.now()
        updateStatus({ status: 'success', endTime })
        callbacks.onStatusChange?.('success')
        callbacks.onSuccess?.(xhr)
        resolve(xhr)
      })

      // 错误处理
      xhr.addEventListener('error', () => {
        const endTime = Date.now()
        let errorMessage = ''

        if (xhr.status === 0) {
          // 状态码 0 通常表示网络连接失败、CORS 问题或服务器不可达
          errorMessage = t('upload.runtimeError.networkFailed')
        } else if (xhr.status >= 400 && xhr.status < 500) {
          errorMessage = t('upload.runtimeError.clientError', { status: xhr.status })
        } else if (xhr.status >= 500) {
          errorMessage = t('upload.runtimeError.serverError', { status: xhr.status })
        } else {
          errorMessage = t('upload.runtimeError.networkError', { status: xhr.status })
        }

        // 检查是否可以重试
        const canRetry =
          attempt < maxRetries &&
          (xhr.status === 0 || // 网络错误
            xhr.status >= 500 || // 服务器错误
            xhr.status === 429) // 频率限制

        if (canRetry) {
          updateStatus({
            status: 'error',
            error: t('upload.runtimeError.retrying', {
              message: errorMessage,
              attempt,
              max: maxRetries,
            }),
            endTime,
          })
          callbacks.onRetry?.(attempt, maxRetries)

          // 延迟后重试
          setTimeout(() => {
            uploadAlone(file, signedUrl, callbacks, attempt + 1)
              .then(resolve)
              .catch(reject)
          }, retryDelay * attempt) // 指数退避
        } else {
          updateStatus({ status: 'error', error: errorMessage, endTime })
          callbacks.onStatusChange?.('error')
          callbacks.onError?.(errorMessage, xhr)
          reject(new Error(errorMessage))
        }
      })

      // 超时处理
      xhr.addEventListener('timeout', () => {
        const endTime = Date.now()
        const errorMessage = t('upload.runtimeError.timeout', { timeout })

        // 检查是否可以重试（超时也可以重试）
        const canRetry = attempt < maxRetries

        if (canRetry) {
          updateStatus({
            status: 'error',
            error: t('upload.runtimeError.retrying', {
              message: errorMessage,
              attempt,
              max: maxRetries,
            }),
            endTime,
          })
          callbacks.onRetry?.(attempt, maxRetries)

          // 延迟后重试
          setTimeout(() => {
            uploadAlone(file, signedUrl, callbacks, attempt + 1)
              .then(resolve)
              .catch(reject)
          }, retryDelay * attempt) // 指数退避
        } else {
          updateStatus({ status: 'error', error: errorMessage, endTime })
          callbacks.onStatusChange?.('error')
          callbacks.onError?.(errorMessage, xhr)
          reject(new Error(errorMessage))
        }
      })

      // 中止处理
      xhr.addEventListener('abort', () => {
        const endTime = Date.now()
        updateStatus({ status: 'aborted', endTime })
        callbacks.onStatusChange?.('aborted')
        callbacks.onAbort?.()
        reject(new Error(t('upload.runtimeError.aborted')))
      })

      // 状态变化处理
      xhr.addEventListener('readystatechange', () => {
        if (xhr.readyState === XMLHttpRequest.DONE) {
          if (xhr.status >= 200 && xhr.status < 300) {
            // 成功情况在 load 事件中处理
            return
          } else if (xhr.status >= 400) {
            const endTime = Date.now()
            let errorMessage = ''

            // 根据状态码提供更友好的错误信息
            switch (xhr.status) {
              case 400:
                errorMessage = t('upload.runtimeError.badRequest')
                break
              case 401:
                errorMessage = t('upload.runtimeError.unauthorized')
                break
              case 403:
                errorMessage = t('upload.runtimeError.forbidden')
                break
              case 404:
                errorMessage = t('upload.runtimeError.notFound')
                break
              case 409:
                errorMessage = t('upload.runtimeError.conflict')
                break
              case 413:
                errorMessage = t('upload.runtimeError.fileTooLarge')
                break
              case 415:
                errorMessage = t('upload.runtimeError.unsupportedType')
                break
              case 429:
                errorMessage = t('upload.runtimeError.rateLimited')
                break
              case 500:
                errorMessage = t('upload.runtimeError.internalServerError')
                break
              case 502:
              case 503:
              case 504:
                errorMessage = t('upload.runtimeError.serviceUnavailable')
                break
              default:
                errorMessage = t('upload.runtimeError.httpError', { status: xhr.status })
            }

            // 尝试获取服务器返回的详细错误信息
            try {
              const responseText = xhr.responseText
              if (responseText) {
                try {
                  JSON.parse(responseText)
                  errorMessage = responseErrorMessage(xhr, errorMessage)
                } catch {
                  // 如果不是 JSON，使用原始文本
                  errorMessage += ` - ${responseText}`
                }
              }
            } catch {
              // 忽略解析错误
            }

            updateStatus({ status: 'error', error: errorMessage, endTime })
            callbacks.onStatusChange?.('error')
            callbacks.onError?.(errorMessage, xhr)
            reject(new Error(errorMessage))
          }
        }
      })

      // 准备请求
      xhr.open('PUT', signedUrl)

      // 设置请求头
      xhr.setRequestHeader(
        'Content-Type',
        file.type || 'application/octet-stream',
      )

      // 设置自定义请求头
      Object.entries(headers).forEach(([key, value]) => {
        xhr.setRequestHeader(key, value)
      })

      // 开始上传
      xhr.send(file)
    })
  }

  // How many parts of one file travel at the same time. Three keeps a fast
  // connection busy without turning one upload into a flood of requests.
  const partConcurrency = 3

  /**
   * Sends a file the server asked to receive in parts. The parts go out a few
   * at a time and each one is retried on its own, so a connection that drops
   * one part does not restart the file. Only once every part is in does the
   * upload finish — the wait for that is what `finalizing` reports.
   */
  const uploadInParts = async (
    file: File,
    plan: UploadPlan,
    callbacks: UploadCallbacks,
  ): Promise<XMLHttpRequest | undefined> => {
    resetStatus()
    const size = file.size
    const partUrls = plan.partUrls ?? []
    const sent = Array.from({ length: plan.partCount }, () => 0)
    const pending = Array.from({ length: plan.partCount }, (_, index) => index)
    let reported = 0

    // Progress is measured from the bytes of every part together, and reported
    // at most a few times a second: three streams of events would otherwise
    // re-render the queue on every chunk.
    const report = (now = false) => {
      const stamp = Date.now()
      if (!now && stamp - reported < 250) return
      reported = stamp
      const loaded = sent.reduce((total, bytes) => total + bytes, 0)
      updateProgress(Math.min(loaded, size), size)
      callbacks.onProgress?.(uploadStatus.value.progress)
    }

    const send = (index: number, attempt: number): Promise<void> =>
      new Promise((resolve, reject) => {
        const partUrl =
          plan.mode === 'multipart'
            ? partUrls[index]
            : `${plan.url}?index=${index}`
        if (!partUrl) {
          reject(new Error(t('upload.error.uploadFailed.message')))
          return
        }
        const start = index * plan.partSize
        const body = file.slice(start, Math.min(start + plan.partSize, size))
        const xhr = new XMLHttpRequest()
        partXHRs.add(xhr)

        const retry = (message: string) => {
          partXHRs.delete(xhr)
          if (attempt < maxRetries && !cancelled) {
            // The part starts over, so what it had sent no longer counts.
            sent[index] = 0
            report(true)
            setTimeout(() => {
              send(index, attempt + 1)
                .then(resolve)
                .catch(reject)
            }, retryDelay * attempt)
            return
          }
          reject(new Error(message))
        }

        if (timeout > 0) {
          xhr.timeout = timeout
        }
        xhr.withCredentials = withCredentials

        xhr.upload.addEventListener('progress', (event) => {
          sent[index] = event.loaded
          report()
        })

        xhr.addEventListener('load', () => {
          partXHRs.delete(xhr)
          if (xhr.status >= 200 && xhr.status < 300) {
            sent[index] = body.size
            report(true)
            resolve()
            return
          }
          const message = responseErrorMessage(
            xhr,
            t('upload.runtimeError.clientError', { status: xhr.status }),
          )
          if (xhr.status >= 500 || xhr.status === 429) {
            retry(message)
            return
          }
          reject(new Error(message))
        })

        xhr.addEventListener('error', () => {
          retry(t('upload.runtimeError.networkFailed'))
        })

        xhr.addEventListener('timeout', () => {
          retry(t('upload.runtimeError.timeout', { timeout }))
        })

        xhr.addEventListener('abort', () => {
          partXHRs.delete(xhr)
          reject(new Error(t('upload.runtimeError.aborted')))
        })

        xhr.open('PUT', partUrl)
        if (plan.mode === 'chunks') {
          // The parts of a file the server assembles carry no content type of
          // their own; the file's own type is what the finished upload is
          // stored under.
          xhr.setRequestHeader('Content-Type', 'application/octet-stream')
        }
        xhr.send(body)
      })

    // Finishing asks the server (or the object store, through it) to join the
    // parts. Nothing is left to send, so a failure here is worth retrying.
    const finish = (attempt: number): Promise<XMLHttpRequest | undefined> =>
      new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest()
        partXHRs.add(xhr)
        if (timeout > 0) {
          xhr.timeout = timeout
        }
        xhr.withCredentials = withCredentials

        const settle = () => {
          partXHRs.delete(xhr)
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(xhr)
            return
          }
          const message = responseErrorMessage(
            xhr,
            xhr.status === 0
              ? t('upload.runtimeError.networkFailed')
              : t('upload.runtimeError.serverError', { status: xhr.status }),
          )
          if (
            attempt < maxRetries &&
            (xhr.status === 0 || xhr.status >= 500 || xhr.status === 429)
          ) {
            setTimeout(() => {
              finish(attempt + 1)
                .then(resolve)
                .catch(reject)
            }, retryDelay * attempt)
            return
          }
          reject(new Error(message))
        }

        xhr.addEventListener('load', settle)
        xhr.addEventListener('error', settle)
        xhr.addEventListener('timeout', settle)
        xhr.addEventListener('abort', () => {
          partXHRs.delete(xhr)
          reject(new Error(t('upload.runtimeError.aborted')))
        })

        xhr.open('POST', plan.url)
        xhr.send()
      })

    // Giving up tells the server to throw away the parts it is holding. The
    // server collects an upload nobody is sending any more on its own, so a
    // failure here is not worth troubling the person who gave up.
    const discard = async () => {
      try {
        await $fetch(plan.url, { method: 'DELETE' })
      } catch {
        /* the server will collect the abandoned upload by itself */
      }
    }

    const startTime = Date.now()
    updateProgress(0, size)
    updateStatus({ status: 'uploading', startTime })
    callbacks.onStatusChange?.('uploading')
    partsRunning = true
    cancelled = false
    halted = false

    try {
      await Promise.all(
        Array.from(
          { length: Math.min(partConcurrency, plan.partCount) },
          async () => {
            while (pending.length > 0 && !cancelled && !halted) {
              const index = pending.shift()
              if (index === undefined) return
              await send(index, 1)
            }
          },
        ),
      )
      if (cancelled) {
        throw new Error(t('upload.runtimeError.aborted'))
      }

      partsRunning = false
      updateStatus({ status: 'finalizing' })
      callbacks.onStatusChange?.('finalizing')
      const finished = await finish(1)

      updateStatus({ status: 'success', endTime: Date.now() })
      callbacks.onStatusChange?.('success')
      callbacks.onSuccess?.(finished)
      return finished
    } catch (error) {
      const endTime = Date.now()
      partsRunning = false
      // Stop the parts that are still in flight and the ones not started yet;
      // an upload that failed on one part is not helped by the rest.
      halted = !cancelled
      pending.length = 0
      for (const xhr of partXHRs) xhr.abort()
      partXHRs.clear()
      void discard()

      const message =
        error instanceof Error && error.message
          ? error.message
          : t('upload.error.uploadFailed.message')
      if (cancelled) {
        updateStatus({ status: 'aborted', endTime })
        callbacks.onStatusChange?.('aborted')
        callbacks.onAbort?.()
        throw new Error(t('upload.runtimeError.aborted'))
      }
      updateStatus({ status: 'error', error: message, endTime })
      callbacks.onStatusChange?.('error')
      callbacks.onError?.(message)
      throw new Error(message)
    }
  }

  /**
   * Uploads a file the way the server asked for it: in one request when the
   * response carried no plan, otherwise in parts.
   */
  const uploadFile = async (
    file: File,
    target: string | UploadPlan,
    callbacks: UploadCallbacks = {},
    attempt: number = 1,
  ): Promise<XMLHttpRequest | undefined> => {
    if (typeof target === 'string') {
      return uploadAlone(file, target, callbacks, attempt)
    }
    return uploadInParts(file, target, callbacks)
  }

  // 中止上传
  const abortUpload = () => {
    if (uploadStatus.value.status !== 'uploading') return
    if (currentXHR) {
      currentXHR.abort()
    }
    if (partsRunning) {
      cancelled = true
      for (const xhr of partXHRs) xhr.abort()
    }
  }

  // 格式化字节大小
  const formatBytes = (bytes: number): string => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  // 格式化时间
  const formatTime = (seconds: number): string => {
    if (!isFinite(seconds) || seconds < 0) return t('upload.progress.calculating')

    const hours = Math.floor(seconds / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    const secs = Math.floor(seconds % 60)

    if (hours > 0) {
      return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`
    } else if (minutes > 0) {
      return `${minutes}:${secs.toString().padStart(2, '0')}`
    } else {
      return t('upload.progress.seconds', { seconds: secs })
    }
  }

  // 计算属性
  const isUploading = computed(() => uploadStatus.value.status === 'uploading')
  const isIdle = computed(() => uploadStatus.value.status === 'idle')
  const isSuccess = computed(() => uploadStatus.value.status === 'success')
  const isError = computed(() => uploadStatus.value.status === 'error')
  const isAborted = computed(() => uploadStatus.value.status === 'aborted')
  const canAbort = computed(
    () => isUploading.value && (currentXHR !== null || partsRunning),
  )

  // 格式化的进度信息
  const formattedProgress = computed(() => {
    const { loaded, total, percentage, speed, timeRemaining } =
      uploadStatus.value.progress
    return {
      percentage,
      loadedText: formatBytes(loaded),
      totalText: formatBytes(total),
      speedText: speed ? `${formatBytes(speed)}/s` : '',
      timeRemainingText: timeRemaining ? formatTime(timeRemaining) : '',
      progressText: `${formatBytes(loaded)} / ${formatBytes(total)} (${percentage}%)`,
    }
  })

  // 清理函数（仅在组件上下文中可用）
  try {
    onUnmounted(() => {
      abortUpload()
      currentXHR = null
    })
  } catch {
    /* empty */
  }

  return {
    uploadStatus: readonly(uploadStatus),

    isUploading,
    isIdle,
    isSuccess,
    isError,
    isAborted,
    canAbort,
    formattedProgress,

    uploadFile,
    abortUpload,
    resetStatus,
    formatBytes,
    formatTime,
  }
}
