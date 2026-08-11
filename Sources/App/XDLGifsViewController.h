#import <Cocoa/Cocoa.h>
#import "XDLPageViewController.h"

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

/// Batch-convert downloaded GIF mp4s to real .gif files via FFmpeg.
@interface XDLGifsViewController : XDLPageViewController

- (instancetype)initWithClient:(XDLCoreClient *)client;

@end

NS_ASSUME_NONNULL_END
