type ExifValue = string | number | boolean | null

export interface NeededExif {
  Title?: string
  XPTitle?: string
  Subject?: string[]
  Keywords?: string[]
  XPKeywords?: string

  Description?: ExifValue
  ImageDescription?: ExifValue
  CaptionAbstract?: ExifValue
  XPComment?: ExifValue
  UserComment?: ExifValue

  zone?: string
  tz?: string
  tzSource?: string

  Orientation?: number
  Make?: string
  Model?: string
  Software?: string
  Artist?: string
  Copyright?: string

  ExposureTime?: string | number
  FNumber?: number
  ExposureProgram?: string
  ISO?: number
  ShutterSpeedValue?: string | number
  ApertureValue?: number
  BrightnessValue?: number
  ExposureCompensation?: number
  MaxApertureValue?: number

  OffsetTime?: string
  OffsetTimeOriginal?: string
  OffsetTimeDigitized?: string

  LightSource?: string
  Flash?: string

  FocalLength?: string
  FocalLengthIn35mmFormat?: string

  LensMake?: string
  LensModel?: string

  ColorSpace?: string

  ExposureMode?: string
  SceneCaptureType?: string

  Aperture?: number
  ScaleFactor35efl?: number
  ShutterSpeed?: string | number
  LightValue?: number

  DateTimeOriginal?: string
  DateTimeDigitized?: string

  ImageWidth?: number
  ImageHeight?: number

  MeteringMode?: ExifValue
  WhiteBalance?: ExifValue
  WBShiftAB?: ExifValue
  WBShiftGM?: ExifValue
  WhiteBalanceBias?: ExifValue
  WhiteBalanceFineTune?: ExifValue
  FlashMeteringMode?: ExifValue
  SensingMethod?: ExifValue
  FocalPlaneXResolution?: ExifValue
  FocalPlaneYResolution?: ExifValue
  GPSAltitude?: ExifValue
  GPSLatitude?: ExifValue
  GPSLongitude?: ExifValue
  GPSAltitudeRef?: ExifValue
  GPSLatitudeRef?: ExifValue
  GPSLongitudeRef?: ExifValue

  // HDR Type
  MPImageType?: ExifValue

  Rating?: number

  // Motion Photo (XMP) related fields
  MotionPhoto?: ExifValue
  MotionPhotoVersion?: ExifValue
  MotionPhotoPresentationTimestampUs?: ExifValue
  MicroVideo?: ExifValue
  MicroVideoVersion?: ExifValue
  MicroVideoOffset?: ExifValue
  MicroVideoPresentationTimestampUs?: ExifValue
}

export interface PhotoInfo {
  title: string
  dateTaken: string
  tags: string[]
  description: string
}
