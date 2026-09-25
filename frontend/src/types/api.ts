export interface PageMeta {
  page: number
  page_size: number
  total: number
}

export interface ApiEnvelope<T> {
  data: T
  meta?: PageMeta
  request_id: string
}
