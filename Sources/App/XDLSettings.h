#import <Foundation/Foundation.h>

NS_ASSUME_NONNULL_BEGIN

/// Posted whenever any setting changes (used to sync preferences to the core).
extern NSNotificationName const XDLSettingsChangedNotification;

/// NSUserDefaults-backed app settings (the old app kept these in webview
/// localStorage).
@interface XDLSettings : NSObject

@property (class, nonatomic, readonly) XDLSettings *shared;

@property (nonatomic, copy) NSString *downloadPath; // empty = core default (~/Pictures)
@property (nonatomic) NSInteger concurrentDownloads; // 1–50
@property (nonatomic) NSInteger retryAttempts;       // 1–5
@property (nonatomic) BOOL skipExistingFiles;
@property (nonatomic) BOOL deleteIncompleteFiles;
@property (nonatomic, copy) NSString *proxy;
@property (nonatomic) NSInteger fetchTimeout; // seconds per extraction page, 10–600

@property (nonatomic) BOOL includePhotos;
@property (nonatomic) BOOL includeVideos;
@property (nonatomic) BOOL includeGifs;
@property (nonatomic) BOOL includeText;
@property (nonatomic) BOOL includeRetweets;
@property (nonatomic, copy) NSString *videoQuality; // highest|720|480|360|lowest
@property (nonatomic, copy) NSString *imageSize;    // orig|large|medium|small

@property (nonatomic, copy) NSString *filenameTemplate;
@property (nonatomic, copy) NSString *folderTemplate;

@property (nonatomic) BOOL autoConvertGifs;
@property (nonatomic, copy) NSString *gifQuality;    // fast|better
@property (nonatomic, copy) NSString *gifResolution; // original|high|medium|low

/// Snapshot for the core's POST /settings (browser-initiated jobs use it).
- (NSDictionary *)coreSyncPayload;

@end

NS_ASSUME_NONNULL_END
