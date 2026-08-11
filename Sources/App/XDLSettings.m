#import "XDLSettings.h"

NSNotificationName const XDLSettingsChangedNotification = @"XDLSettingsChangedNotification";

static void NotifyChanged(void) {
    [NSNotificationCenter.defaultCenter postNotificationName:XDLSettingsChangedNotification object:nil];
}

@implementation XDLSettings

+ (XDLSettings *)shared {
    static XDLSettings *instance;
    static dispatch_once_t once;
    dispatch_once(&once, ^{
        instance = [[XDLSettings alloc] init];
        [NSUserDefaults.standardUserDefaults registerDefaults:@{
            @"downloadPath": @"",
            @"concurrentDownloads": @10,
            @"retryAttempts": @2,
            @"skipExistingFiles": @YES,
            @"deleteIncompleteFiles": @YES,
            @"proxy": @"",
            @"fetchTimeout": @60,
            @"includePhotos": @YES,
            @"includeVideos": @YES,
            @"includeGifs": @YES,
            @"includeText": @NO,
            @"includeRetweets": @NO,
            @"videoQuality": @"highest",
            @"imageSize": @"orig",
            @"filenameTemplate": @"{handle}_{date}_{tweet_id}",
            @"folderTemplate": @"{handle}",
            @"autoConvertGifs": @NO,
            @"gifQuality": @"fast",
            @"gifResolution": @"original",
        }];
    });
    return instance;
}

static NSString *StringValue(NSString *key) {
    return [NSUserDefaults.standardUserDefaults stringForKey:key] ?: @"";
}

static void SetString(NSString *key, NSString *value) {
    [NSUserDefaults.standardUserDefaults setObject:value forKey:key];
    NotifyChanged();
}

static BOOL BoolValue(NSString *key) {
    return [NSUserDefaults.standardUserDefaults boolForKey:key];
}

static void SetBool(NSString *key, BOOL value) {
    [NSUserDefaults.standardUserDefaults setBool:value forKey:key];
    NotifyChanged();
}

static NSInteger IntValue(NSString *key) {
    return [NSUserDefaults.standardUserDefaults integerForKey:key];
}

static void SetInt(NSString *key, NSInteger value) {
    [NSUserDefaults.standardUserDefaults setInteger:value forKey:key];
    NotifyChanged();
}

- (NSString *)downloadPath { return StringValue(@"downloadPath"); }
- (void)setDownloadPath:(NSString *)value { SetString(@"downloadPath", value); }

- (NSInteger)concurrentDownloads { return MAX(1, MIN(50, IntValue(@"concurrentDownloads"))); }
- (void)setConcurrentDownloads:(NSInteger)value { SetInt(@"concurrentDownloads", MAX(1, MIN(50, value))); }

- (NSInteger)retryAttempts { return MAX(1, MIN(5, IntValue(@"retryAttempts"))); }
- (void)setRetryAttempts:(NSInteger)value { SetInt(@"retryAttempts", MAX(1, MIN(5, value))); }

- (BOOL)skipExistingFiles { return BoolValue(@"skipExistingFiles"); }
- (void)setSkipExistingFiles:(BOOL)value { SetBool(@"skipExistingFiles", value); }

- (BOOL)deleteIncompleteFiles { return BoolValue(@"deleteIncompleteFiles"); }
- (void)setDeleteIncompleteFiles:(BOOL)value { SetBool(@"deleteIncompleteFiles", value); }

- (NSString *)proxy { return StringValue(@"proxy"); }
- (void)setProxy:(NSString *)value { SetString(@"proxy", value); }

- (NSInteger)fetchTimeout { return MAX(10, MIN(600, IntValue(@"fetchTimeout"))); }
- (void)setFetchTimeout:(NSInteger)value { SetInt(@"fetchTimeout", MAX(10, MIN(600, value))); }

- (BOOL)includePhotos { return BoolValue(@"includePhotos"); }
- (void)setIncludePhotos:(BOOL)value { SetBool(@"includePhotos", value); }

- (BOOL)includeVideos { return BoolValue(@"includeVideos"); }
- (void)setIncludeVideos:(BOOL)value { SetBool(@"includeVideos", value); }

- (BOOL)includeGifs { return BoolValue(@"includeGifs"); }
- (void)setIncludeGifs:(BOOL)value { SetBool(@"includeGifs", value); }

- (BOOL)includeText { return BoolValue(@"includeText"); }
- (void)setIncludeText:(BOOL)value { SetBool(@"includeText", value); }

- (BOOL)includeRetweets { return BoolValue(@"includeRetweets"); }
- (void)setIncludeRetweets:(BOOL)value { SetBool(@"includeRetweets", value); }

- (NSString *)videoQuality { return StringValue(@"videoQuality"); }
- (void)setVideoQuality:(NSString *)value { SetString(@"videoQuality", value); }

- (NSString *)imageSize { return StringValue(@"imageSize"); }
- (void)setImageSize:(NSString *)value { SetString(@"imageSize", value); }

- (NSString *)filenameTemplate { return StringValue(@"filenameTemplate"); }
- (void)setFilenameTemplate:(NSString *)value { SetString(@"filenameTemplate", value); }

- (NSString *)folderTemplate { return StringValue(@"folderTemplate"); }
- (void)setFolderTemplate:(NSString *)value { SetString(@"folderTemplate", value); }

- (BOOL)autoConvertGifs { return BoolValue(@"autoConvertGifs"); }
- (void)setAutoConvertGifs:(BOOL)value { SetBool(@"autoConvertGifs", value); }

- (NSString *)gifQuality { return StringValue(@"gifQuality"); }
- (void)setGifQuality:(NSString *)value { SetString(@"gifQuality", value); }

- (NSString *)gifResolution { return StringValue(@"gifResolution"); }
- (void)setGifResolution:(NSString *)value { SetString(@"gifResolution", value); }

- (NSDictionary *)coreSyncPayload {
    return @{
        @"download_path": self.downloadPath,
        @"concurrent_downloads": @(self.concurrentDownloads),
        @"skip_existing": self.skipExistingFiles ? @YES : @NO,
        @"delete_incomplete_files": self.deleteIncompleteFiles ? @YES : @NO,
        @"retry_attempts": @(self.retryAttempts),
        @"proxy": self.proxy,
        @"filename_template": self.filenameTemplate,
        @"folder_template": self.folderTemplate,
        @"auto_convert_gifs": self.autoConvertGifs ? @YES : @NO,
        @"gif_quality": self.gifQuality,
        @"gif_resolution": self.gifResolution,
        @"include_photos": self.includePhotos ? @YES : @NO,
        @"include_videos": self.includeVideos ? @YES : @NO,
        @"include_gifs": self.includeGifs ? @YES : @NO,
        @"include_text": self.includeText ? @YES : @NO,
        @"retweets": self.includeRetweets ? @YES : @NO,
        @"video_quality": self.videoQuality,
        @"image_size": self.imageSize,
    };
}

@end
