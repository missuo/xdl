#import <Cocoa/Cocoa.h>
#import "XDLPageViewController.h"

NS_ASSUME_NONNULL_BEGIN

@class XDLCoreClient;

/// xtractor / FFmpeg / ExifTool install status and downloads.
@interface XDLDependenciesViewController : XDLPageViewController

- (instancetype)initWithClient:(XDLCoreClient *)client;

@end

NS_ASSUME_NONNULL_END
