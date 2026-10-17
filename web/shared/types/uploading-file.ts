import type { UploadQueueStatus } from '~~/shared/utils/upload-status'
import type { UploadPlan } from '~~/shared/types/upload'

/**
 * One file on the upload queue, as the dashboard shows it: the file itself, how
 * far it has got, and the state the queue panel renders from.
 */
export interface UploadingFile {
  file: File
  fileName: string
  fileId: string
  status: UploadQueueStatus
  stage?: string | null
  progress?: number
  error?: string
  warning?: string
  taskId?: number
  signedUrlResponse?: {
    signedUrl: string
    fileKey: string
    expiresIn: number
    /**
     * How a large file should be sent. The server offers it in place of the
     * single request every smaller file still uses.
     */
    upload?: UploadPlan
  }
  uploadProgress?: {
    loaded: number
    total: number
    percentage: number
    speed?: number
    timeRemaining?: number
    speedText?: string
    timeRemainingText?: string
  }
  canAbort?: boolean
  abortUpload?: () => void
}
