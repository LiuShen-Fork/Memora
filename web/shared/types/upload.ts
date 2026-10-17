/**
 * How a prepared upload should travel.
 *
 * The server decides this when a file is prepared, because it is the only side
 * that knows which storage the site uses. There is no `single` mode: a response
 * without a plan carries `signedUrl` alone, and that is a file sent whole.
 */
export type UploadPlanMode = 'chunks' | 'multipart'

export interface UploadPlan {
  mode: UploadPlanMode
  /** Where the parts are sent, and where the upload is finished or given up. */
  url: string
  partSize: number
  partCount: number
  /**
   * One signed URL per part, for an upload that goes straight to the object
   * store. A plan that the server assembles itself has none: its parts all
   * travel to `url`, numbered by a query parameter.
   */
  partUrls?: string[]
}

/** The plan a prepared upload carries, or nothing when it is sent whole. */
export interface PreparedUpload {
  signedUrl: string
  fileKey: string
  expiresIn: number
  upload?: UploadPlan
}
